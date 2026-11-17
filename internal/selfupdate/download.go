package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strings"
	"time"
)

const (
	maxMetadataSize = 1 << 20
	maxArtifactSize = 512 << 20
	maxBinarySize   = 256 << 20
	downloadTimeout = 3 * time.Minute
)

func (m *Manager) download(ctx context.Context, target, source string) ([]byte, string, error) {
	if source != "auto" && source != "github" {
		return nil, "", fmt.Errorf("标准安装包仅支持 GitHub Release，收到来源 %q", source)
	}
	artifact, err := m.downloadReleaseAsset(ctx, target)
	if err != nil {
		return nil, "", err
	}
	return artifact, "github", nil
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
