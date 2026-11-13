package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"evergate/internal/update"
)

const (
	npmScope        = "@heguoliang"
	maxMetadataSize = 1 << 20
	maxChecksumSize = 1 << 20
	maxTarballSize  = 256 << 20
	maxBinarySize   = 256 << 20
	downloadTimeout = 3 * time.Minute
)

func (m *Manager) download(ctx context.Context, target, source string) ([]byte, string, error) {
	switch source {
	case "npm":
		binary, err := m.downloadNPM(ctx, target)
		return binary, "npm", err
	case "github":
		binary, err := m.downloadGitHub(ctx, target)
		return binary, "github", err
	default:
		binary, err := m.downloadNPM(ctx, target)
		if err == nil {
			return binary, "npm", nil
		}
		m.opt.Logger.Warn("npm update channel failed, falling back to GitHub",
			"error", err, "target", target)
		binary, ghErr := m.downloadGitHub(ctx, target)
		if ghErr != nil {
			return nil, "", fmt.Errorf("npm: %v; github: %w", err, ghErr)
		}
		return binary, "github", nil
	}
}

type npmVersionMeta struct {
	Dist struct {
		Tarball   string `json:"tarball"`
		Integrity string `json:"integrity"`
	} `json:"dist"`
}

func (m *Manager) downloadNPM(ctx context.Context, target string) ([]byte, error) {
	pkg, binPath, err := platformPackage(m.opt.GOOS, m.opt.GOARCH)
	if err != nil {
		return nil, err
	}
	metaURL := m.opt.Registry + "/" + pkg + "/" + url.PathEscape(target)
	body, err := httpGet(ctx, m.opt.Client, metaURL, maxMetadataSize)
	if err != nil {
		return nil, fmt.Errorf("查询 npm 包 %s@%s 失败: %w", pkg, target, err)
	}
	var meta npmVersionMeta
	if err := json.Unmarshal(body, &meta); err != nil {
		return nil, fmt.Errorf("解析 npm 元数据失败: %w", err)
	}
	if meta.Dist.Tarball == "" || meta.Dist.Integrity == "" {
		return nil, errors.New("npm 元数据缺少 tarball 或 integrity")
	}
	parsed, err := url.Parse(meta.Dist.Tarball)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return nil, fmt.Errorf("npm tarball 地址不合法: %q", meta.Dist.Tarball)
	}
	tarball, err := httpGet(ctx, m.opt.Client, meta.Dist.Tarball, maxTarballSize)
	if err != nil {
		return nil, fmt.Errorf("下载 npm tarball 失败: %w", err)
	}
	if err := verifyNPMIntegrity(tarball, meta.Dist.Integrity); err != nil {
		return nil, err
	}
	binary, err := extractBinary(tarball, binPath)
	if err != nil {
		return nil, err
	}
	return binary, nil
}

func (m *Manager) downloadGitHub(ctx context.Context, target string) ([]byte, error) {
	if m.opt.Latest == nil {
		return nil, errors.New("GitHub 回退通道未配置")
	}
	status, err := m.opt.Latest(ctx)
	if err != nil {
		return nil, fmt.Errorf("查询 GitHub Release 失败: %w", err)
	}
	if status.Development {
		return nil, errors.New("开发版本无法使用 GitHub 回退通道")
	}
	if update.CompareVersions(status.LatestVersion, target) != 0 {
		return nil, fmt.Errorf("GitHub 回退仅支持最新版本 %s，目标版本为 %s", status.LatestVersion, target)
	}
	assetName, err := githubBinaryName(m.opt.GOOS, m.opt.GOARCH)
	if err != nil {
		return nil, err
	}
	var binaryURL, checksumURL string
	for _, asset := range status.Assets {
		switch asset.Name {
		case assetName:
			binaryURL = asset.URL
		case "checksums.txt":
			checksumURL = asset.URL
		}
	}
	if binaryURL == "" || checksumURL == "" {
		return nil, fmt.Errorf("Release 缺少 %s 或 checksums.txt", assetName)
	}
	checksums, err := httpGet(ctx, m.opt.Client, checksumURL, maxChecksumSize)
	if err != nil {
		return nil, fmt.Errorf("下载 checksums.txt 失败: %w", err)
	}
	binary, err := httpGet(ctx, m.opt.Client, binaryURL, maxBinarySize)
	if err != nil {
		return nil, fmt.Errorf("下载 Release 二进制失败: %w", err)
	}
	if err := verifyChecksum(binary, checksums, assetName); err != nil {
		return nil, err
	}
	return binary, nil
}

func httpGet(ctx context.Context, client *http.Client, rawURL string, max int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "EverGate-SelfUpdate")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("HTTP %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > max {
		return nil, fmt.Errorf("响应超过 %d 字节上限", max)
	}
	return body, nil
}

func verifyNPMIntegrity(tarball []byte, integrity string) error {
	for _, field := range strings.Fields(integrity) {
		if !strings.HasPrefix(field, "sha512-") {
			continue
		}
		want := strings.TrimPrefix(field, "sha512-")
		sum := sha512.Sum512(tarball)
		got := base64.StdEncoding.EncodeToString(sum[:])
		if got != want {
			return errors.New("npm tarball integrity 校验失败")
		}
		return nil
	}
	return errors.New("npm 元数据未提供 sha512 integrity")
}

func extractBinary(tarball []byte, expectedPath string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(tarball))
	if err != nil {
		return nil, fmt.Errorf("解压 npm tarball 失败: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("读取 npm tarball 失败: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg || hdr.Name != expectedPath {
			continue
		}
		if hdr.Size > maxBinarySize {
			return nil, errors.New("npm 包内二进制超过大小上限")
		}
		binary, err := io.ReadAll(io.LimitReader(tr, maxBinarySize+1))
		if err != nil {
			return nil, err
		}
		if int64(len(binary)) > maxBinarySize {
			return nil, errors.New("npm 包内二进制超过大小上限")
		}
		return binary, nil
	}
	return nil, fmt.Errorf("npm 包内未找到 %s", expectedPath)
}

func verifyChecksum(binary, checksums []byte, assetName string) error {
	target := filepath.Base(assetName)
	for _, line := range strings.Split(string(checksums), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		file := filepath.Base(fields[len(fields)-1])
		if file != target {
			continue
		}
		want := strings.ToLower(fields[0])
		sum := sha256.Sum256(binary)
		got := hex.EncodeToString(sum[:])
		if got != want {
			return errors.New("Release 二进制 SHA256 校验失败")
		}
		return nil
	}
	return fmt.Errorf("checksums.txt 中未找到 %s", target)
}

func platformPackage(goos, goarch string) (pkg, binPath string, err error) {
	switch goos + "-" + goarch {
	case "windows-amd64":
		return npmScope + "/evergate-win32-x64", "package/bin/evergate.exe", nil
	case "linux-amd64":
		return npmScope + "/evergate-linux-amd64", "package/bin/evergate", nil
	case "darwin-arm64":
		return npmScope + "/evergate-darwin-arm64", "package/bin/evergate", nil
	default:
		return "", "", fmt.Errorf("当前平台 %s/%s 不支持自动更新", goos, goarch)
	}
}

func githubBinaryName(goos, goarch string) (string, error) {
	switch goos + "-" + goarch {
	case "windows-amd64":
		return "evergate-windows-amd64.exe", nil
	case "linux-amd64":
		return "evergate-linux-amd64", nil
	case "darwin-arm64":
		return "evergate-darwin-arm64", nil
	default:
		return "", fmt.Errorf("当前平台 %s/%s 不支持自动更新", goos, goarch)
	}
}
