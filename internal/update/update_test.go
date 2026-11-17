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
		{current: "v0.2.0-rc.2", latest: "v0.2.0-rc.10", want: -1},
		{current: "v0.2.0-beta.9", latest: "v0.2.0-rc.1", want: -1},
		{current: "v0.2.0-rc.1", latest: "v0.2.0", want: -1},
		{current: "v0.2.0", latest: "v0.2.0-rc.1", want: 1},
		{current: "dev", latest: "v0.1.4", want: 0},
	}
	for _, tt := range tests {
		if got := CompareVersions(tt.current, tt.latest); got != tt.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", tt.current, tt.latest, got, tt.want)
		}
	}
}

func TestDevelopmentVersionForms(t *testing.T) {
	tests := []struct {
		version string
		want    bool
	}{
		{version: "", want: true},
		{version: "dev", want: true},
		{version: "v0.1.29-dirty", want: true},
		{version: "v0.1.29-9-g9370b7e", want: true},
		{version: "0.1.29-9-g9370b7e-dirty", want: true},
		{version: "v0.1.29", want: false},
		{version: "v0.2.0-rc.1", want: false},
		{version: "v0.2.0-beta.2", want: false},
	}
	for _, tt := range tests {
		if got := IsDevelopmentVersion(tt.version); got != tt.want {
			t.Errorf("IsDevelopmentVersion(%q) = %v, want %v", tt.version, got, tt.want)
		}
	}
}

func TestPrereleaseEligibility(t *testing.T) {
	tests := []struct {
		current   string
		candidate string
		want      bool
	}{
		{current: "v0.2.0", candidate: "v0.2.1", want: true},
		{current: "v0.2.0", candidate: "v0.2.1-rc.1", want: false},
		{current: "v0.2.0-rc.1", candidate: "v0.2.0-rc.2", want: true},
		{current: "v0.2.0-rc.1", candidate: "v0.2.0", want: true},
		{current: "v0.2.0-rc.1", candidate: "v0.2.0-beta.2", want: false},
		{current: "v0.2.0-beta.1", candidate: "v0.2.0-beta.2", want: true},
	}
	for _, tt := range tests {
		if got := isVersionEligible(tt.current, tt.candidate); got != tt.want {
			t.Errorf("isVersionEligible(%q, %q) = %v, want %v", tt.current, tt.candidate, got, tt.want)
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
	for _, version := range []string{"dev", "v0.1.29-dirty", "v0.1.29-9-g9370b7e"} {
		checker := NewChecker(version, "http://127.0.0.1:1/nope", time.Now)
		status, err := checker.Check(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if status.Development != true || status.UpdateAvailable || status.LatestVersion != "" {
			t.Fatalf("version %q produced unexpected development status: %+v", version, status)
		}
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
	if len(status.Assets) != 0 {
		t.Fatalf("atom fallback must not invent legacy binary assets: %+v", status.Assets)
	}
}

func TestAtomFallbackSelectsStableReleaseForStableVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/guol9180/evergate/releases/latest":
			w.WriteHeader(http.StatusForbidden)
		case "/releases.atom":
			_, _ = w.Write([]byte(`<?xml version="1.0"?><feed>
				<entry><title>v0.3.0-rc.1</title><link href="https://example/rc"/><updated>2026-11-12T01:00:00Z</updated></entry>
				<entry><title>v0.2.0</title><link href="https://example/stable"/><updated>2026-11-11T01:00:00Z</updated></entry>
			</feed>`))
		default:
			t.Errorf("path = %q", r.URL.Path)
		}
	}))
	defer server.Close()
	checker := NewCheckerWithOptions(Options{
		CurrentVersion: "v0.1.29",
		APIURL:         server.URL + "/repos/guol9180/evergate/releases/latest",
		Registries:     []string{},
		Now:            time.Now,
	})
	status, err := checker.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !status.UpdateAvailable || status.LatestVersion != "v0.2.0" || status.ReleaseURL != "https://example/stable" {
		t.Fatalf("status = %+v", status)
	}
}

func TestAtomFallbackIgnoresPrereleaseOnlyForStableVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/guol9180/evergate/releases/latest":
			w.WriteHeader(http.StatusForbidden)
		case "/releases.atom":
			_, _ = w.Write([]byte(`<?xml version="1.0"?><feed>
				<entry><title>v0.3.0-rc.1</title><link href="https://example/rc"/><updated>2026-11-12T01:00:00Z</updated></entry>
			</feed>`))
		default:
			t.Errorf("path = %q", r.URL.Path)
		}
	}))
	defer server.Close()
	checker := NewCheckerWithOptions(Options{
		CurrentVersion: "v0.1.29",
		APIURL:         server.URL + "/repos/guol9180/evergate/releases/latest",
		Registries:     []string{},
		Now:            time.Now,
	})
	status, err := checker.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.UpdateAvailable || status.LatestVersion != "" || status.Source != "github" {
		t.Fatalf("status = %+v", status)
	}
}

func npmRegistryServer(t *testing.T, version string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/@heguoliang/evergate/latest" && r.URL.Path != "/@heguoliang/evergate/next" {
			http.NotFound(w, r)
			return
		}
		_, _ = fmt.Fprintf(w, `{"version":%q}`, version)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func npmDistTagServer(t *testing.T, distTag, version string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/@heguoliang/evergate/"+distTag {
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

func TestStableVersionIgnoresNPMPrerelease(t *testing.T) {
	npm := npmRegistryServer(t, "0.2.0-rc.1")
	githubHits := 0
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		githubHits++
		http.Error(w, "should not be called", http.StatusInternalServerError)
	}))
	defer gh.Close()
	checker := NewCheckerWithOptions(Options{
		CurrentVersion: "v0.1.29", APIURL: gh.URL, Registries: []string{npm.URL}, Now: time.Now,
	})
	status, err := checker.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.UpdateAvailable || status.LatestVersion != "" || status.Development || githubHits != 0 {
		t.Fatalf("status = %+v githubHits = %d", status, githubHits)
	}
}

func TestPrereleaseVersionAcceptsSameChannelUpgrade(t *testing.T) {
	npm := npmDistTagServer(t, "next", "0.2.0-rc.2")
	gh := githubReleaseServer(t, "v0.2.0-rc.2", "https://example/release")
	checker := NewCheckerWithOptions(Options{
		CurrentVersion: "v0.2.0-rc.1", APIURL: gh.URL, Registries: []string{npm.URL}, Now: time.Now,
	})
	status, err := checker.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !status.UpdateAvailable || status.LatestVersion != "0.2.0-rc.2" ||
		status.ReleaseURL != "https://example/release" {
		t.Fatalf("status = %+v", status)
	}
}

func TestPrereleaseVersionPrefersNewerStableLatest(t *testing.T) {
	npm := npmRegistryServer(t, "0.2.0")
	gh := githubReleaseServer(t, "v0.2.0", "https://example/release")
	checker := NewCheckerWithOptions(Options{
		CurrentVersion: "v0.2.0-rc.2", APIURL: gh.URL, Registries: []string{npm.URL}, Now: time.Now,
	})
	status, err := checker.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !status.UpdateAvailable || status.LatestVersion != "0.2.0" {
		t.Fatalf("status = %+v", status)
	}
}

func TestStableVersionIgnoresGitHubPrereleaseFallback(t *testing.T) {
	npm := npmRegistryServer(t, "not-a-version")
	gh := githubReleaseServer(t, "v0.2.0-rc.1", "https://example/release")
	checker := NewCheckerWithOptions(Options{
		CurrentVersion: "v0.1.29", APIURL: gh.URL, Registries: []string{npm.URL}, Now: time.Now,
	})
	status, err := checker.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.UpdateAvailable || status.LatestVersion != "" || status.Source != "github" {
		t.Fatalf("status = %+v", status)
	}
}
