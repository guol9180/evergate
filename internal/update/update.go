// Package update checks EverGate releases published on GitHub.
package update

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultAPIURL  = "https://api.github.com/repos/guol9180/evergate/releases/latest"
	DefaultAtomURL = "https://github.com/guol9180/evergate/releases.atom"
	userAgent      = "EverGate-Update-Checker"
)

var versionPattern = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z.-]+))?$`)

type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

type Status struct {
	CurrentVersion  string    `json:"current_version"`
	LatestVersion   string    `json:"latest_version,omitempty"`
	ReleaseURL      string    `json:"release_url,omitempty"`
	PublishedAt     time.Time `json:"published_at,omitempty"`
	UpdateAvailable bool      `json:"update_available"`
	Development     bool      `json:"development,omitempty"`
	CheckedAt       time.Time `json:"checked_at"`
	Assets          []Asset   `json:"assets,omitempty"`
}

type Checker struct {
	currentVersion string
	apiURL         string
	now            func() time.Time
	client         *http.Client
}

type githubRelease struct {
	TagName     string    `json:"tag_name"`
	HTMLURL     string    `json:"html_url"`
	PublishedAt time.Time `json:"published_at"`
	Assets      []Asset   `json:"assets"`
}

var errRateLimited = errors.New("update endpoint rate limited")

func NewChecker(currentVersion, apiURL string, now func() time.Time) *Checker {
	if apiURL == "" {
		apiURL = DefaultAPIURL
	}
	if now == nil {
		now = time.Now
	}
	return &Checker{
		currentVersion: currentVersion, apiURL: apiURL, now: now,
		client: &http.Client{Timeout: 4 * time.Second},
	}
}

func (c *Checker) Check(ctx context.Context) (Status, error) {
	status := Status{CurrentVersion: c.currentVersion, CheckedAt: c.now()}
	if c.currentVersion == "" || c.currentVersion == "dev" || strings.HasSuffix(c.currentVersion, "-dirty") {
		status.Development = true
		return status, nil
	}
	release, err := c.fetchAPI(ctx)
	if errors.Is(err, errRateLimited) {
		release, err = c.fetchAtom(ctx)
	}
	if err != nil {
		return status, err
	}
	if !versionPattern.MatchString(release.TagName) {
		return status, fmt.Errorf("invalid release tag %q", release.TagName)
	}
	status.LatestVersion = release.TagName
	status.ReleaseURL = release.HTMLURL
	status.PublishedAt = release.PublishedAt
	status.Assets = release.Assets
	status.UpdateAvailable = CompareVersions(c.currentVersion, release.TagName) < 0
	return status, nil
}

func (c *Checker) fetchAPI(ctx context.Context) (githubRelease, error) {
	var release githubRelease
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiURL, nil)
	if err != nil {
		return release, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.client.Do(req)
	if err != nil {
		return release, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return release, errRateLimited
	}
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return release, fmt.Errorf("update endpoint returned %s", resp.Status)
	}
	return release, json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&release)
}

func (c *Checker) fetchAtom(ctx context.Context) (githubRelease, error) {
	var feed struct {
		Entries []struct {
			Title string `xml:"title"`
			Link  struct {
				Href string `xml:"href,attr"`
			} `xml:"link"`
			Updated time.Time `xml:"updated"`
		} `xml:"entry"`
	}
	atomURL := DefaultAtomURL
	if c.apiURL != DefaultAPIURL {
		atomURL = strings.TrimSuffix(c.apiURL, "/repos/guol9180/evergate/releases/latest") + "/releases.atom"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, atomURL, nil)
	if err != nil {
		return githubRelease{}, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.client.Do(req)
	if err != nil {
		return githubRelease{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return githubRelease{}, fmt.Errorf("update fallback returned %s", resp.Status)
	}
	if err := xml.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&feed); err != nil {
		return githubRelease{}, err
	}
	if len(feed.Entries) == 0 {
		return githubRelease{}, errors.New("update fallback has no releases")
	}
	entry := feed.Entries[0]
	return githubRelease{
		TagName: entry.Title, HTMLURL: entry.Link.Href, PublishedAt: entry.Updated,
		Assets: []Asset{{
			Name: "evergate-windows-amd64.exe",
			URL:  fmt.Sprintf("https://github.com/guol9180/evergate/releases/download/%s/evergate-windows-amd64.exe", entry.Title),
		}},
	}, nil
}

func CompareVersions(current, latest string) int {
	if current == "dev" || current == "" {
		return 0
	}
	a, ok := parseVersion(current)
	if !ok {
		return 0
	}
	b, ok := parseVersion(latest)
	if !ok {
		return 0
	}
	for i := range a.numeric {
		if a.numeric[i] != b.numeric[i] {
			if a.numeric[i] < b.numeric[i] {
				return -1
			}
			return 1
		}
	}
	switch {
	case a.pre == b.pre:
		return 0
	case a.pre == "":
		return 1
	case b.pre == "":
		return -1
	case a.pre < b.pre:
		return -1
	default:
		return 1
	}
}

type parsedVersion struct {
	numeric [3]int
	pre     string
}

func parseVersion(v string) (parsedVersion, bool) {
	m := versionPattern.FindStringSubmatch(strings.TrimSpace(v))
	if m == nil {
		return parsedVersion{}, false
	}
	var out parsedVersion
	for i := 0; i < 3; i++ {
		n, err := strconv.Atoi(m[i+1])
		if err != nil {
			return parsedVersion{}, false
		}
		out.numeric[i] = n
	}
	out.pre = m[4]
	return out, true
}
