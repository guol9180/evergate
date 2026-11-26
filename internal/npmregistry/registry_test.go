package npmregistry

import (
	"reflect"
	"testing"
)

func TestDefaultChain(t *testing.T) {
	want := []string{Mirror, Official}
	if got := Default(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Default() = %v, want %v", got, want)
	}
}

func TestNormalizeTrimsDeduplicatesAndKeepsOrder(t *testing.T) {
	in := []string{"  https://a.example/  ", "", "https://b.example", "https://a.example", "   ", "https://b.example/"}
	want := []string{"https://a.example", "https://b.example"}
	if got := Normalize(in); !reflect.DeepEqual(got, want) {
		t.Fatalf("Normalize() = %v, want %v", got, want)
	}
	if got := Normalize(nil); len(got) != 0 {
		t.Fatalf("Normalize(nil) = %v, want empty", got)
	}
}

func TestHost(t *testing.T) {
	cases := map[string]string{
		"https://registry.example.com/path": "registry.example.com",
		"https://registry.example.com":      "registry.example.com",
		"not a url":                         "not a url",
		"":                                  "",
	}
	for in, want := range cases {
		if got := Host(in); got != want {
			t.Fatalf("Host(%q) = %q, want %q", in, got, want)
		}
	}
}
