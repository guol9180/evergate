package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"

	"evergate/internal/store"
)

func TestListModelsProtocolsAndPagination(t *testing.T) {
	t.Run("anthropic", func(t *testing.T) {
		var calls int
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			if r.URL.Path != "/v1/models" ||
				r.Header.Get("x-api-key") != "sk-ant" ||
				r.Header.Get("anthropic-version") != "2023-06-01" ||
				r.URL.Query().Get("limit") != "100" {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			switch calls {
			case 1:
				_, _ = w.Write([]byte(`{"data":[{"id":"claude-a"}],"has_more":true,"last_id":"claude-a"}`))
			case 2:
				if r.URL.Query().Get("after_id") != "claude-a" {
					http.Error(w, "missing after_id", http.StatusBadRequest)
					return
				}
				_, _ = w.Write([]byte(`{"data":[{"id":"claude-b"}],"has_more":false}`))
			default:
				http.Error(w, "unexpected page", http.StatusInternalServerError)
			}
		}))
		t.Cleanup(server.Close)

		result, err := ListModels(context.Background(), server.Client(), &store.Channel{
			BaseURL: server.URL, SupportsAnthropic: true,
		}, "sk-ant")
		if err != nil {
			t.Fatalf("list Anthropic models: %v", err)
		}
		if !reflect.DeepEqual(result.Models, []string{"claude-a", "claude-b"}) || result.Pages != 2 || result.Protocol != "anthropic" {
			t.Fatalf("result = %#v", result)
		}
	})

	t.Run("gemini", func(t *testing.T) {
		var calls int
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			if r.URL.Path != "/v1beta/models" ||
				r.Header.Get("x-goog-api-key") != "sk-gem" ||
				r.URL.Query().Get("pageSize") != "100" ||
				r.URL.Query().Get("key") != "" {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			switch calls {
			case 1:
				_, _ = w.Write([]byte(`{"models":[{"name":"models/gemini-a"}],"nextPageToken":"next-page"}`))
			case 2:
				if r.URL.Query().Get("pageToken") != "next-page" {
					http.Error(w, "missing pageToken", http.StatusBadRequest)
					return
				}
				_, _ = w.Write([]byte(`{"models":[{"name":"models/gemini-b"}]}`))
			default:
				http.Error(w, "unexpected page", http.StatusInternalServerError)
			}
		}))
		t.Cleanup(server.Close)

		result, err := ListModels(context.Background(), server.Client(), &store.Channel{
			BaseURL: server.URL, SupportsGemini: true,
		}, "sk-gem")
		if err != nil {
			t.Fatalf("list Gemini models: %v", err)
		}
		if !reflect.DeepEqual(result.Models, []string{"gemini-a", "gemini-b"}) || result.Pages != 2 || result.Protocol != "gemini" {
			t.Fatalf("result = %#v", result)
		}
	})

	t.Run("openai-compatible", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/models" || r.Header.Get("Authorization") != "Bearer sk-open" {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"id":"gpt-a"},{"id":"gpt-a"},{"id":"gpt-b"}]}`))
		}))
		t.Cleanup(server.Close)

		result, err := ListModels(context.Background(), server.Client(), &store.Channel{BaseURL: server.URL}, "sk-open")
		if err != nil {
			t.Fatalf("list OpenAI-compatible models: %v", err)
		}
		if !reflect.DeepEqual(result.Models, []string{"gpt-a", "gpt-b"}) || result.Pages != 1 || result.Protocol != "openai-compatible" {
			t.Fatalf("result = %#v", result)
		}
	})
}

func TestListModelsEnforcesLimits(t *testing.T) {
	var mu sync.Mutex
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		page := calls
		mu.Unlock()
		data := make([]map[string]string, 0, 100)
		for i := 0; i < 100; i++ {
			data = append(data, map[string]string{"id": fmt.Sprintf("m-%d-%d", page, i)})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": data, "has_more": true, "last_id": "last-" + strconv.Itoa(page),
		})
	}))
	t.Cleanup(server.Close)

	result, err := ListModels(context.Background(), server.Client(), &store.Channel{
		BaseURL: server.URL, SupportsAnthropic: true,
	}, "sk-ant")
	if err != nil {
		t.Fatalf("list models: %v", err)
	}
	if len(result.Models) != maxModels || !result.Truncated {
		t.Fatalf("models=%d truncated=%v, want %d/true", len(result.Models), result.Truncated, maxModels)
	}
	mu.Lock()
	gotCalls := calls
	mu.Unlock()
	if gotCalls >= maxPages {
		t.Fatalf("model cap should stop before page cap, calls=%d", gotCalls)
	}
}

func TestListModelsJSONErrorDoesNotLeakKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "invalid credential sk-secret", http.StatusUnauthorized)
	}))
	t.Cleanup(server.Close)

	_, err := ListModels(context.Background(), server.Client(), &store.Channel{BaseURL: server.URL}, "sk-secret")
	if err == nil || !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(err.Error(), "sk-secret") {
		t.Fatalf("error leaked key: %v", err)
	}
}

func TestListModelsUsesOpenCodeGoCatalogEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/inference/go/v1/models" {
			t.Errorf("path = %q, want OpenCode catalog endpoint", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer sk-opencode" {
			t.Errorf("Authorization = %q", auth)
		}
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"glm-5.3-flash"},{"id":"gpt-6-luna"}]}`))
	}))
	defer server.Close()

	result, err := ListModels(context.Background(), server.Client(), &store.Channel{
		BaseURL: server.URL + "/inference/go/openai/v1",
	}, "sk-opencode")
	if err != nil {
		t.Fatalf("list OpenCode models: %v", err)
	}
	if !reflect.DeepEqual(result.Models, []string{"glm-5.3-flash", "gpt-6-luna"}) {
		t.Fatalf("models = %#v", result.Models)
	}
}

func TestListModelsParsesOpenRouterStylePricing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"m-priced","pricing":{"prompt":"0.0000015","completion":"0.000006"}},{"id":"m-cache","pricing":{"prompt":"0.000003","completion":"0.000015","input_cache_read":"0.0000003","input_cache_write":"0.00000375"}},{"id":"m-free","pricing":{"prompt":"0","completion":"0"}},{"id":"m-plain"}]}`))
	}))
	t.Cleanup(server.Close)
	result, err := ListModels(context.Background(), server.Client(), &store.Channel{BaseURL: server.URL}, "sk")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	price, ok := result.Prices["m-priced"]
	if !ok || price.InputMicrosPerMillion != 1_500_000 || price.OutputMicrosPerMillion != 6_000_000 {
		t.Fatalf("pricing = %#v %v, want 1.5/6 USD per million", price, ok)
	}
	if price.CacheReadMicrosPerMillion != 0 || price.CacheWriteMicrosPerMillion != 0 {
		t.Fatalf("pricing without cache fields = %#v, want zero cache prices", price)
	}
	cached, ok := result.Prices["m-cache"]
	if !ok || cached.CacheReadMicrosPerMillion != 300_000 || cached.CacheWriteMicrosPerMillion != 3_750_000 {
		t.Fatalf("cache pricing = %#v %v, want read 0.3 / write 3.75 USD per million", cached, ok)
	}
	if _, ok := result.Prices["m-free"]; ok {
		t.Fatal("zero price must not be offered as a catalog price")
	}
	if _, ok := result.Prices["m-plain"]; ok {
		t.Fatal("model without pricing must not appear in Prices")
	}
	if len(result.Models) != 4 {
		t.Fatalf("models = %#v", result.Models)
	}
}
