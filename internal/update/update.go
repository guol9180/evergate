// Package update 检查 EverGate 最新版本：优先 npm dist-tags（npmmirror → 官方
// registry），两源均不可用时回退 GitHub Releases；GitHub 可达时补充更新说明与资产。
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

	"evergate/internal/npmregistry"
)

const (
	DefaultAPIURL   = "https://api.github.com/repos/guol9180/evergate/releases/latest"
	DefaultAtomURL  = "https://github.com/guol9180/evergate/releases.atom"
	userAgent       = "EverGate-Update-Checker"
	maxResponseSize = 1 << 20

	defaultNPMTimeout    = 3 * time.Second
	defaultGitHubTimeout = 3 * time.Second
)

var versionPattern = regexp.MustCompile(`^v?(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$`)

var developmentVersionPattern = regexp.MustCompile(`^v?(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)-\d+-g[0-9a-fA-F]+(?:-dirty)?$`)

type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

type Status struct {
	CurrentVersion  string     `json:"current_version"`
	LatestVersion   string     `json:"latest_version,omitempty"`
	ReleaseURL      string     `json:"release_url,omitempty"`
	PublishedAt     *time.Time `json:"published_at,omitempty"`
	UpdateAvailable bool       `json:"update_available"`
	Development     bool       `json:"development,omitempty"`
	CheckedAt       time.Time  `json:"checked_at"`
	Assets          []Asset    `json:"assets,omitempty"`
	// Source 记录版本信息来源：npm:<host> 或 github。
	Source string `json:"source,omitempty"`
}

type Checker struct {
	currentVersion string
	apiURL         string
	atomURL        string
	registries     []string
	pkg            string
	now            func() time.Time
	client         *http.Client
	npmTimeout     time.Duration
	githubTimeout  time.Duration
}

type githubRelease struct {
	TagName     string    `json:"tag_name"`
	HTMLURL     string    `json:"html_url"`
	PublishedAt time.Time `json:"published_at"`
	Assets      []Asset   `json:"assets"`
}

var (
	errRateLimited       = errors.New("update endpoint rate limited")
	errNoEligibleRelease = errors.New("update fallback has no eligible releases")
)

// Options 是版本检查器装配依赖。
// Registries 为 nil 时使用 npmmirror → 官方 registry；传非 nil 空切片可关闭 npm 检查。
type Options struct {
	CurrentVersion string
	APIURL         string
	AtomURL        string
	Registries     []string
	Package        string
	Now            func() time.Time
	Client         *http.Client
	NPMTimeout     time.Duration
	GitHubTimeout  time.Duration
}

func NewCheckerWithOptions(opt Options) *Checker {
	apiURL := opt.APIURL
	if apiURL == "" {
		apiURL = DefaultAPIURL
	}
	atomURL := opt.AtomURL
	if atomURL == "" {
		atomURL = DefaultAtomURL
		if apiURL != DefaultAPIURL {
			atomURL = strings.TrimSuffix(apiURL, "/repos/guol9180/evergate/releases/latest") + "/releases.atom"
		}
	}
	now := opt.Now
	if now == nil {
		now = time.Now
	}
	registries := opt.Registries
	if registries == nil {
		registries = npmregistry.Default()
	}
	registries = npmregistry.Normalize(registries)
	pkg := opt.Package
	if pkg == "" {
		pkg = npmregistry.Package
	}
	npmTimeout := opt.NPMTimeout
	if npmTimeout <= 0 {
		npmTimeout = defaultNPMTimeout
	}
	githubTimeout := opt.GitHubTimeout
	if githubTimeout <= 0 {
		githubTimeout = defaultGitHubTimeout
	}
	client := opt.Client
	if client == nil {
		client = &http.Client{Timeout: npmTimeout + githubTimeout + time.Second}
	}
	return &Checker{
		currentVersion: opt.CurrentVersion, apiURL: apiURL, atomURL: atomURL,
		registries: registries, pkg: pkg, now: now, client: client,
		npmTimeout: npmTimeout, githubTimeout: githubTimeout,
	}
}

func (c *Checker) Check(ctx context.Context) (Status, error) {
	status := Status{CurrentVersion: c.currentVersion, CheckedAt: c.now()}
	if IsDevelopmentVersion(c.currentVersion) {
		status.Development = true
		return status, nil
	}

	latest, source, npmErr := c.checkNPM(ctx)
	if npmErr != nil {
		release, err := c.fetchRelease(ctx)
		if err != nil {
			if errors.Is(err, errNoEligibleRelease) {
				status.Source = "github"
				return status, nil
			}
			return status, fmt.Errorf("npm: %v; github: %w", npmErr, err)
		}
		if !versionPattern.MatchString(release.TagName) {
			return status, fmt.Errorf("invalid release tag %q", release.TagName)
		}
		status.Source = "github"
		applyRelease(&status, release)
		return status, nil
	}

	status.Source = source
	if !isVersionEligible(c.currentVersion, latest) {
		return status, nil
	}
	status.LatestVersion = latest
	status.UpdateAvailable = CompareVersions(c.currentVersion, latest) < 0
	if !status.UpdateAvailable {
		return status, nil
	}

	// GitHub 仅用于补充更新说明与资产；不可达时不影响 npm 已确认的版本。
	enrichCtx, cancel := context.WithTimeout(ctx, c.githubTimeout)
	release, err := c.fetchRelease(enrichCtx)
	cancel()
	if err != nil || !versionPattern.MatchString(release.TagName) {
		return status, nil
	}
	switch CompareVersions(release.TagName, latest) {
	case 0:
		status.ReleaseURL = release.HTMLURL
		status.PublishedAt = &release.PublishedAt
		status.Assets = release.Assets
	case 1:
		// GitHub 版本更新说明 npm 尚未同步（或 npm 发布失败），以 GitHub 为准。
		if applyRelease(&status, release) {
			status.Source = "github"
		}
	}
	return status, nil
}

func applyRelease(status *Status, release githubRelease) bool {
	if !isVersionEligible(status.CurrentVersion, release.TagName) {
		return false
	}
	status.LatestVersion = release.TagName
	status.ReleaseURL = release.HTMLURL
	status.PublishedAt = &release.PublishedAt
	status.Assets = release.Assets
	status.UpdateAvailable = CompareVersions(status.CurrentVersion, release.TagName) < 0
	return true
}

// checkNPM 按 chain 顺序读取 npm dist-tags，返回版本与来源标识。
func (c *Checker) checkNPM(ctx context.Context) (version, source string, err error) {
	failures := make([]string, 0, len(c.registries))
	for _, registry := range c.registries {
		version, err := c.fetchCurrentNPMVersion(ctx, registry)
		if err == nil {
			return version, "npm:" + npmregistry.Host(registry), nil
		}
		failures = append(failures, fmt.Sprintf("%s: %v", npmregistry.Host(registry), err))
		if ctx.Err() != nil {
			break
		}
	}
	if len(failures) == 0 {
		return "", "", errors.New("npm 版本检查未启用或 registry 为空")
	}
	return "", "", fmt.Errorf("npm（%s）", strings.Join(failures, "；"))
}

func (c *Checker) fetchCurrentNPMVersion(ctx context.Context, registry string) (string, error) {
	latest, latestErr := c.fetchNPMVersion(ctx, registry, "latest")
	if !isPrereleaseVersion(c.currentVersion) {
		return latest, latestErr
	}
	next, nextErr := c.fetchNPMVersion(ctx, registry, "next")
	versions := []string{latest, next}
	best := ""
	for _, candidate := range versions {
		if !isVersionEligible(c.currentVersion, candidate) {
			continue
		}
		if best == "" || CompareVersions(best, candidate) < 0 {
			best = candidate
		}
	}
	if best != "" {
		return best, nil
	}
	return "", fmt.Errorf("latest: %v; next: %v", latestErr, nextErr)
}

func (c *Checker) fetchNPMVersion(ctx context.Context, registry, distTag string) (string, error) {
	reqCtx, cancel := context.WithTimeout(ctx, c.npmTimeout)
	defer cancel()
	endpoint := registry + "/" + c.pkg + "/" + distTag
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("npm registry returned %s", resp.Status)
	}
	var meta struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseSize)).Decode(&meta); err != nil {
		return "", fmt.Errorf("解析 npm 版本元数据失败: %w", err)
	}
	if !versionPattern.MatchString(meta.Version) {
		return "", fmt.Errorf("invalid npm version %q", meta.Version)
	}
	return meta.Version, nil
}

func (c *Checker) fetchRelease(ctx context.Context) (githubRelease, error) {
	apiCtx, cancelAPI := context.WithTimeout(ctx, c.githubTimeout)
	release, err := c.fetchAPI(apiCtx)
	cancelAPI()
	if errors.Is(err, errRateLimited) {
		atomCtx, cancelAtom := context.WithTimeout(ctx, c.githubTimeout)
		defer cancelAtom()
		return c.fetchAtom(atomCtx)
	}
	return release, err
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
	return release, json.NewDecoder(io.LimitReader(resp.Body, maxResponseSize)).Decode(&release)
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.atomURL, nil)
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
	if err := xml.NewDecoder(io.LimitReader(resp.Body, maxResponseSize)).Decode(&feed); err != nil {
		return githubRelease{}, err
	}
	if len(feed.Entries) == 0 {
		return githubRelease{}, errors.New("update fallback has no releases")
	}
	for _, entry := range feed.Entries {
		if !versionPattern.MatchString(entry.Title) || !isVersionEligible(c.currentVersion, entry.Title) {
			continue
		}
		return githubRelease{
			TagName: entry.Title, HTMLURL: entry.Link.Href, PublishedAt: entry.Updated,
		}, nil
	}
	return githubRelease{}, errNoEligibleRelease
}

func CompareVersions(current, latest string) int {
	if IsDevelopmentVersion(current) {
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
	case len(a.pre) == 0 && len(b.pre) == 0:
		return 0
	case len(a.pre) == 0:
		return 1
	case len(b.pre) == 0:
		return -1
	}
	return comparePrerelease(a.pre, b.pre)
}

type parsedVersion struct {
	numeric [3]int
	pre     []prereleaseIdentifier
}

type prereleaseIdentifier struct {
	text    string
	numeric bool
	number  int
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
	if m[4] == "" {
		return out, true
	}
	parts := strings.Split(m[4], ".")
	out.pre = make([]prereleaseIdentifier, 0, len(parts))
	for _, part := range parts {
		identifier := prereleaseIdentifier{text: part}
		if isDigits(part) {
			if len(part) > 1 && part[0] == '0' {
				return parsedVersion{}, false
			}
			n, err := strconv.Atoi(part)
			if err != nil {
				return parsedVersion{}, false
			}
			identifier.numeric = true
			identifier.number = n
		}
		out.pre = append(out.pre, identifier)
	}
	return out, true
}

// IsDevelopmentVersion 识别未发布构建；这些版本不参与更新比较。
func IsDevelopmentVersion(version string) bool {
	version = strings.TrimSpace(version)
	return version == "" || version == "dev" || strings.HasSuffix(version, "-dirty") ||
		developmentVersionPattern.MatchString(version)
}

func isVersionEligible(current, candidate string) bool {
	latest, ok := parseVersion(candidate)
	if !ok {
		return false
	}
	if len(latest.pre) == 0 {
		return true
	}
	installed, ok := parseVersion(current)
	if !ok || len(installed.pre) == 0 {
		return false
	}
	return installed.pre[0].text == latest.pre[0].text
}

func isPrereleaseVersion(version string) bool {
	parsed, ok := parseVersion(version)
	return ok && len(parsed.pre) > 0
}

func comparePrerelease(a, b []prereleaseIdentifier) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		switch {
		case a[i].text == b[i].text:
			continue
		case a[i].numeric && b[i].numeric:
			if a[i].number < b[i].number {
				return -1
			}
			return 1
		case a[i].numeric:
			return -1
		case b[i].numeric:
			return 1
		case a[i].text < b[i].text:
			return -1
		default:
			return 1
		}
	}
	switch {
	case len(a) < len(b):
		return -1
	case len(a) > len(b):
		return 1
	default:
		return 0
	}
}

func isDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}
