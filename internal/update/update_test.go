package update

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"evergate/internal/npmregistry"
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

	checker := NewCheckerWithOptions(Options{
		CurrentVersion: "v0.1.3",
		APIURL:         server.URL + "/repos/guol9180/evergate/releases/latest",
		Registries:     []string{},
		Now:            func() time.Time { return now },
	})
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
	if status.Source != "github" {
		t.Fatalf("source = %q, want github", status.Source)
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
	checker := NewCheckerWithOptions(Options{CurrentVersion: "v0.1.3", APIURL: server.URL, Registries: []string{}, Now: time.Now})
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
	checker := NewCheckerWithOptions(Options{
		CurrentVersion: "v0.1.3",
		APIURL:         server.URL + "/repos/guol9180/evergate/releases/latest",
		Registries:     []string{},
		Now:            time.Now,
	})
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

func npmRegistryServer(t *testing.T, version string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/@heguoliang/evergate/latest" {
			http.NotFound(w, r)
			return
		}
		_, _ = fmt.Fprintf(w, `{"version":%q}`, version)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func githubReleaseServer(t *testing.T, tag, htmlURL string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `{"tag_name":%q,"html_url":%q,"published_at":"2026-11-16T00:00:00Z","assets":[{"name":"evergate-windows-amd64.exe","browser_download_url":%q,"size":123}]}`, tag, htmlURL, htmlURL+"/bin")
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestCheckPrefersNPMAndEnrichesGitHub(t *testing.T) {
	npm := npmRegistryServer(t, "0.1.27")
	gh := githubReleaseServer(t, "v0.1.27", "https://example/release")
	checker := NewCheckerWithOptions(Options{
		CurrentVersion: "v0.1.26", APIURL: gh.URL, Registries: []string{npm.URL}, Now: time.Now,
	})
	status, err := checker.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.LatestVersion != "0.1.27" || !status.UpdateAvailable {
		t.Fatalf("status = %+v", status)
	}
	if want := "npm:" + npmregistry.Host(npm.URL); status.Source != want {
		t.Fatalf("source = %q, want %q", status.Source, want)
	}
	if status.ReleaseURL != "https://example/release" || len(status.Assets) != 1 {
		t.Fatalf("github metadata not enriched: %+v", status)
	}
}

func TestCheckFallsBackToOfficialRegistry(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "mirror down", http.StatusBadGateway)
	}))
	defer bad.Close()
	good := npmRegistryServer(t, "0.1.27")
	checker := NewCheckerWithOptions(Options{
		CurrentVersion: "v0.1.26",
		APIURL:         "http://127.0.0.1:1/unreachable",
		Registries:     []string{bad.URL, good.URL},
		GitHubTimeout:  300 * time.Millisecond,
		Now:            time.Now,
	})
	status, err := checker.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.LatestVersion != "0.1.27" || !status.UpdateAvailable {
		t.Fatalf("status = %+v", status)
	}
	if want := "npm:" + npmregistry.Host(good.URL); status.Source != want {
		t.Fatalf("source = %q, want %q", status.Source, want)
	}
	if status.ReleaseURL != "" || status.Assets != nil {
		t.Fatalf("unreachable github should not add metadata: %+v", status)
	}
}

func TestCheckFallsBackToGitHubWhenNPMFails(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "npm down", http.StatusServiceUnavailable)
	}))
	defer bad.Close()
	gh := githubReleaseServer(t, "v0.1.27", "https://example/release")
	checker := NewCheckerWithOptions(Options{
		CurrentVersion: "v0.1.26", APIURL: gh.URL, Registries: []string{bad.URL}, Now: time.Now,
	})
	status, err := checker.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.LatestVersion != "v0.1.27" || status.Source != "github" || len(status.Assets) != 1 {
		t.Fatalf("status = %+v", status)
	}
}

func TestCheckSkipsGitHubWhenNoUpdate(t *testing.T) {
	npm := npmRegistryServer(t, "0.1.26")
	githubHits := 0
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		githubHits++
		http.Error(w, "should not be called", http.StatusInternalServerError)
	}))
	defer gh.Close()
	checker := NewCheckerWithOptions(Options{
		CurrentVersion: "v0.1.26", APIURL: gh.URL, Registries: []string{npm.URL}, Now: time.Now,
	})
	status, err := checker.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.UpdateAvailable || githubHits != 0 {
		t.Fatalf("status = %+v githubHits = %d", status, githubHits)
	}
}

func TestCheckPrefersNewerGitHubRelease(t *testing.T) {
	npm := npmRegistryServer(t, "0.1.26")
	gh := githubReleaseServer(t, "v0.1.27", "https://example/release")
	checker := NewCheckerWithOptions(Options{
		CurrentVersion: "v0.1.25", APIURL: gh.URL, Registries: []string{npm.URL}, Now: time.Now,
	})
	status, err := checker.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.LatestVersion != "v0.1.27" || status.Source != "github" || !status.UpdateAvailable {
		t.Fatalf("status = %+v", status)
	}
}

func TestCheckFallsBackToGitHubForInvalidNPMVersion(t *testing.T) {
	npm := npmRegistryServer(t, "not-a-version")
	gh := githubReleaseServer(t, "v0.1.27", "https://example/release")
	checker := NewCheckerWithOptions(Options{
		CurrentVersion: "v0.1.26", APIURL: gh.URL, Registries: []string{npm.URL}, Now: time.Now,
	})
	status, err := checker.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.LatestVersion != "v0.1.27" || status.Source != "github" {
		t.Fatalf("status = %+v", status)
	}
}
