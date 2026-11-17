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
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strings"
	"time"

	"evergate/internal/npmregistry"
)

const (
	maxMetadataSize = 1 << 20
	maxArtifactSize = 512 << 20
	maxBinarySize   = 256 << 20
	downloadTimeout = 3 * time.Minute
)

func (m *Manager) download(ctx context.Context, target, source string) ([]byte, string, error) {
	switch source {
	case "npm":
		return m.downloadNPMChain(ctx, target)
	case "github":
		artifact, err := m.downloadReleaseAsset(ctx, target)
		return artifact, "github", err
	case "auto":
		artifact, usedSource, npmErr := m.downloadNPMChain(ctx, target)
		if npmErr == nil {
			return artifact, usedSource, nil
		}
		m.opt.Logger.Warn("npm update package failed, falling back to GitHub", "error", npmErr, "target", target)
		artifact, ghErr := m.downloadReleaseAsset(ctx, target)
		if ghErr != nil {
			return nil, "", fmt.Errorf("%v; github: %w", npmErr, ghErr)
		}
		return artifact, "github", nil
	default:
		return nil, "", fmt.Errorf("未知更新通道 %q", source)
	}
}

type npmVersionMeta struct {
	Dist struct {
		Tarball   string `json:"tarball"`
		Integrity string `json:"integrity"`
	} `json:"dist"`
}

func (m *Manager) downloadNPMChain(ctx context.Context, target string) ([]byte, string, error) {
	var failures []string
	for _, registry := range m.opt.Registries {
		artifact, err := m.downloadNPMFrom(ctx, registry, target)
		if err == nil {
			return artifact, "npm:" + npmregistry.Host(registry), nil
		}
		m.opt.Logger.Warn("npm update source failed",
			slog.String("registry", registry), slog.String("target", target), slog.Any("error", err))
		failures = append(failures, fmt.Sprintf("%s: %v", npmregistry.Host(registry), err))
		if ctx.Err() != nil {
			break
		}
	}
	if len(failures) == 0 {
		return nil, "", errors.New("npm 更新源为空")
	}
	return nil, "", fmt.Errorf("npm（%s）", strings.Join(failures, "；"))
}

func (m *Manager) downloadNPMFrom(ctx context.Context, registry, target string) ([]byte, error) {
	artifactName, checksumName, err := releaseAssetNames(m.opt.GOOS, m.opt.GOARCH, target)
	if err != nil {
		return nil, err
	}
	metaURL := registry + "/" + npmregistry.UpdatesPackage + "/" + url.PathEscape(target)
	body, err := httpGet(ctx, m.opt.Client, metaURL, maxMetadataSize)
	if err != nil {
		return nil, fmt.Errorf("查询 npm 更新包 %s 失败: %w", target, err)
	}
	var meta npmVersionMeta
	if err := json.Unmarshal(body, &meta); err != nil {
		return nil, fmt.Errorf("解析 npm 更新包元数据失败: %w", err)
	}
	if meta.Dist.Tarball == "" || meta.Dist.Integrity == "" {
		return nil, errors.New("npm 更新包缺少 tarball 或 integrity")
	}
	if err := validateAssetURL(meta.Dist.Tarball); err != nil {
		return nil, fmt.Errorf("npm tarball 地址不合法: %w", err)
	}
	tarball, err := httpGet(ctx, m.opt.Client, meta.Dist.Tarball, maxArtifactSize)
	if err != nil {
		return nil, fmt.Errorf("下载 npm 更新包失败: %w", err)
	}
	if err := verifyNPMIntegrity(tarball, meta.Dist.Integrity); err != nil {
		return nil, err
	}
	artifact, checksums, err := extractNPMAssets(tarball, artifactName, checksumName)
	if err != nil {
		return nil, err
	}
	if err := verifyChecksum(artifact, checksums, artifactName); err != nil {
		return nil, err
	}
	return artifact, nil
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
			return errors.New("npm 更新包 integrity 校验失败")
		}
		return nil
	}
	return errors.New("npm 更新包未提供 sha512 integrity")
}

func extractNPMAssets(tarball []byte, artifactName, checksumName string) ([]byte, []byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(tarball))
	if err != nil {
		return nil, nil, fmt.Errorf("解压 npm 更新包失败: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	wanted := map[string]int64{
		"package/artifacts/" + artifactName: maxArtifactSize,
		"package/artifacts/" + checksumName: maxMetadataSize,
	}
	found := map[string][]byte{}
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("读取 npm 更新包失败: %w", err)
		}
		limit, ok := wanted[hdr.Name]
		if !ok {
			continue
		}
		if hdr.Typeflag != tar.TypeReg {
			return nil, nil, fmt.Errorf("npm 更新包中的 %s 不是普通文件", hdr.Name)
		}
		if _, duplicate := found[hdr.Name]; duplicate {
			return nil, nil, fmt.Errorf("npm 更新包包含重复 %s", hdr.Name)
		}
		if hdr.Size < 0 || hdr.Size > limit {
			return nil, nil, fmt.Errorf("npm 更新包中的 %s 大小非法", hdr.Name)
		}
		data, err := io.ReadAll(io.LimitReader(tr, limit+1))
		if err != nil {
			return nil, nil, err
		}
		if int64(len(data)) > limit {
			return nil, nil, fmt.Errorf("npm 更新包中的 %s 超过大小上限", hdr.Name)
		}
		found[hdr.Name] = data
	}
	artifact := found["package/artifacts/"+artifactName]
	checksums := found["package/artifacts/"+checksumName]
	if len(artifact) == 0 || len(checksums) == 0 {
		return nil, nil, fmt.Errorf("npm 更新包缺少 %s 或 %s", artifactName, checksumName)
	}
	return artifact, checksums, nil
}

// downloadReleaseAsset 精确匹配当前版本、平台和 checksums 资产；任何缺失或重名都拒绝。
func (m *Manager) downloadReleaseAsset(ctx context.Context, target string) ([]byte, error) {
	if m.opt.Latest == nil {
		return nil, errors.New("Release 更新通道未配置")
	}
	status, err := m.opt.Latest(ctx)
	if err != nil {
		return nil, fmt.Errorf("查询 GitHub Release 失败: %w", err)
	}
	if status.Development {
		return nil, errors.New("开发版本无法使用 Release 更新通道")
	}
	if got := strings.TrimPrefix(status.LatestVersion, "v"); got != target {
		return nil, fmt.Errorf("Release 版本 %q 与目标版本 %q 不一致", status.LatestVersion, target)
	}
	artifactName, checksumName, err := releaseAssetNames(m.opt.GOOS, m.opt.GOARCH, target)
	if err != nil {
		return nil, err
	}
	var artifactURL, checksumURL string
	seen := map[string]int{}
	for _, asset := range status.Assets {
		seen[asset.Name]++
		switch asset.Name {
		case artifactName:
			artifactURL = asset.URL
		case checksumName:
			checksumURL = asset.URL
		}
	}
	for _, name := range []string{artifactName, checksumName} {
		if seen[name] != 1 {
			return nil, fmt.Errorf("Release 资产 %s 必须精确出现一次（实际 %d 次）", name, seen[name])
		}
	}
	if err := validateAssetURL(artifactURL); err != nil {
		return nil, fmt.Errorf("%s 地址不合法: %w", artifactName, err)
	}
	if err := validateAssetURL(checksumURL); err != nil {
		return nil, fmt.Errorf("%s 地址不合法: %w", checksumName, err)
	}
	checksums, err := httpGet(ctx, m.opt.Client, checksumURL, maxMetadataSize)
	if err != nil {
		return nil, fmt.Errorf("下载 %s 失败: %w", checksumName, err)
	}
	artifact, err := httpGet(ctx, m.opt.Client, artifactURL, maxArtifactSize)
	if err != nil {
		return nil, fmt.Errorf("下载 %s 失败: %w", artifactName, err)
	}
	if err := verifyChecksum(artifact, checksums, artifactName); err != nil {
		return nil, err
	}
	return artifact, nil
}

func validateAssetURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return fmt.Errorf("unsupported scheme %q", parsed.Scheme)
	}
	return nil
}

func releaseAssetNames(goos, goarch, version string) (artifact, checksums string, err error) {
	prefix := "evergate_" + strings.TrimPrefix(version, "v")
	switch goos + "-" + goarch {
	case "windows-amd64":
		return prefix + "_windows_amd64_setup.exe", prefix + "_checksums.txt", nil
	case "darwin-arm64":
		return prefix + "_macos_arm64_setup.dmg", prefix + "_checksums.txt", nil
	case "linux-amd64":
		return prefix + "_linux_amd64.tar.gz", prefix + "_checksums.txt", nil
	default:
		return "", "", fmt.Errorf("当前平台 %s/%s 不支持安装包更新", goos, goarch)
	}
}

func httpGet(ctx context.Context, client *http.Client, rawURL string, max int64) ([]byte, error) {
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		body, retry, err := httpGetOnce(ctx, client, rawURL, max)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry || attempt == 3 || ctx.Err() != nil {
			return nil, err
		}
		timer := time.NewTimer(time.Duration(attempt) * 300 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	return nil, lastErr
}

func httpGetOnce(ctx context.Context, client *http.Client, rawURL string, max int64) ([]byte, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", "EverGate-SelfUpdate")
	resp, err := client.Do(req)
	if err != nil {
		return nil, ctx.Err() == nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		retry := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		return nil, retry, fmt.Errorf("HTTP %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return nil, ctx.Err() == nil, err
	}
	if int64(len(body)) > max {
		return nil, false, fmt.Errorf("响应超过 %d 字节上限", max)
	}
	return body, false, nil
}

func extractLinuxBinary(archive []byte) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("解压 Linux tar.gz 失败: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	found := false
	var binary []byte
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("读取 Linux tar.gz 失败: %w", err)
		}
		clean := path.Clean(hdr.Name)
		if clean != "evergate" {
			continue
		}
		if found {
			return nil, errors.New("Linux tar.gz 包含重复 evergate")
		}
		if hdr.Typeflag != tar.TypeReg {
			return nil, errors.New("Linux tar.gz 中的 evergate 不是普通文件")
		}
		if hdr.Size <= 0 || hdr.Size > maxBinarySize {
			return nil, errors.New("Linux tar.gz 中的 evergate 大小非法")
		}
		found = true
		binary, err = io.ReadAll(io.LimitReader(tr, maxBinarySize+1))
		if err != nil {
			return nil, err
		}
		if int64(len(binary)) > maxBinarySize {
			return nil, errors.New("Linux evergate 超过大小上限")
		}
	}
	if !found {
		return nil, errors.New("Linux tar.gz 缺少 evergate")
	}
	return binary, nil
}

func verifyChecksum(artifact, checksums []byte, assetName string) error {
	target := filepath.Base(assetName)
	matches := 0
	for _, line := range strings.Split(string(checksums), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		file := filepath.Base(fields[len(fields)-1])
		if file != target {
			continue
		}
		matches++
		want := strings.ToLower(fields[0])
		if len(want) != sha256.Size*2 {
			return errors.New("checksums 中 SHA256 长度非法")
		}
		sum := sha256.Sum256(artifact)
		got := hex.EncodeToString(sum[:])
		if got != want {
			return errors.New("Release 安装包 SHA256 校验失败")
		}
	}
	if matches == 0 {
		return fmt.Errorf("checksums 文件中未找到 %s", target)
	}
	if matches != 1 {
		return fmt.Errorf("checksums 文件中 %s 出现 %d 次", target, matches)
	}
	return nil
}
