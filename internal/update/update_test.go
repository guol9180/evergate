package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestVersionCompare(t *testing.T) {
	tests := []struct {
		current string
		latest  string
		want    int
	}{
		{current: "v0.1.3", latest: "v0.1.4", want: -1},
		{current: "v0.1.4", latest: "v0.1.4", want: 0},
		{current: "v0.2.0", latest: "v0.1.9", want: 1},
		{current: "v0.1.10", latest: "v0.1.9", want: 1},
		{current: "dev", latest: "v0.1.4", want: 0},
	}
	for _, tt := range tests {
		if got := CompareVersions(tt.current, tt.latest); got != tt.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", tt.current, tt.latest, got, tt.want)
		}
	}
}

func TestCheckerFetchesLatestRelease(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/guol9180/evergate/releases/latest" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.Header.Get("Accept") != "application/vnd.github+json" || r.Header.Get("User-Agent") == "" {
			t.Errorf("missing GitHub API headers: %+v", r.Header)
		}
		_, _ = w.Write([]byte(`{
			"tag_name":"v0.1.4","name":"v0.1.4","html_url":"https://example/release",
			"published_at":"2026-11-10T01:00:00Z",
			"assets":[{"name":"evergate-windows-amd64.exe","browser_download_url":"https://example/windows.exe"}]
		}`))
	}))
	defer server.Close()

	checker := NewChecker("v0.1.3", server.URL+"/repos/guol9180/evergate/releases/latest", func() time.Time { return now })
	status, err := checker.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !status.UpdateAvailable || status.LatestVersion != "v0.1.4" || status.ReleaseURL != "https://example/release" {
		t.Fatalf("unexpected status: %+v", status)
	}
	if len(status.Assets) != 1 || status.Assets[0].Name != "evergate-windows-amd64.exe" {
		t.Fatalf("unexpected assets: %+v", status.Assets)
	}
	if !status.CheckedAt.Equal(now) {
		t.Fatalf("CheckedAt = %v, want %v", status.CheckedAt, now)
	}
}

func TestCheckerDevelopmentVersionDoesNotReportUpdate(t *testing.T) {
	checker := NewChecker("dev", "http://127.0.0.1:1/nope", time.Now)
	status, err := checker.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.Development != true || status.UpdateAvailable || status.LatestVersion != "" {
		t.Fatalf("unexpected development status: %+v", status)
	}
}

func TestCheckerRejectsInvalidVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"not-a-version","html_url":"https://example/release"}`))
	}))
	defer server.Close()
	checker := NewChecker("v0.1.3", server.URL, time.Now)
	if _, err := checker.Check(context.Background()); err == nil {
		t.Fatal("invalid tag should fail")
	}
}

func TestCheckerFallsBackToAtomWhenRateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/guol9180/evergate/releases/latest":
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"API rate limit exceeded"}`))
		case "/releases.atom":
			_, _ = w.Write([]byte(`<?xml version="1.0"?><feed><entry><title>v0.1.4</title><link href="https://example/release"/><updated>2026-11-10T01:00:00Z</updated></entry></feed>`))
		default:
			t.Errorf("path = %q", r.URL.Path)
		}
	}))
	defer server.Close()
	checker := NewChecker("v0.1.3", server.URL+"/repos/guol9180/evergate/releases/latest", time.Now)
	status, err := checker.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !status.UpdateAvailable || status.LatestVersion != "v0.1.4" || status.ReleaseURL != "https://example/release" {
		t.Fatalf("unexpected fallback status: %+v", status)
	}
	if len(status.Assets) != 1 || status.Assets[0].URL != "https://github.com/guol9180/evergate/releases/download/v0.1.4/evergate-windows-amd64.exe" {
		t.Fatalf("fallback should derive Windows asset: %+v", status.Assets)
	}
}
