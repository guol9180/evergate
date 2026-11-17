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
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"evergate/internal/npmregistry"
	"evergate/internal/update"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func buildTarball(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{
			Name: name, Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg,
		}); err != nil {
			t.Fatalf("tar header: %v", err)
		}
		if _, err := tw.Write(content); err != nil {
			t.Fatalf("tar write: %v", err)
		}
	}
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}

func checksumFile(name string, data []byte) []byte {
	sum := sha256.Sum256(data)
	return []byte(fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), name))
}

func sha512Integrity(data []byte) string {
	sum := sha512.Sum512(data)
	return "sha512-" + base64.StdEncoding.EncodeToString(sum[:])
}

func npmUpdateServer(t *testing.T, target string, tarball []byte, integrity string) *httptest.Server {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/@heguoliang/evergate-updates/" + target:
			_, _ = fmt.Fprintf(w, `{"dist":{"tarball":%q,"integrity":%q}}`, server.URL+"/updates.tgz", integrity)
		case "/updates.tgz":
			_, _ = w.Write(tarball)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func updatePackageTarball(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	all := map[string][]byte{
		"package/README.md": []byte("readme"),
	}
	for name, content := range files {
		all["package/artifacts/"+name] = content
	}
	return buildTarball(t, all)
}

func TestHTTPGetRetriesTransientServerErrors(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) < 3 {
			http.Error(w, "temporary upstream failure", http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte("checksums"))
	}))
	defer server.Close()

	body, err := httpGet(context.Background(), server.Client(), server.URL, 1024)
	if err != nil {
		t.Fatalf("httpGet: %v", err)
	}
	if string(body) != "checksums" || attempts.Load() != 3 {
		t.Fatalf("body=%q attempts=%d, want success on third attempt", body, attempts.Load())
	}
}

func TestHTTPGetDoesNotRetryClientError(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		http.NotFound(w, r)
	}))
	defer server.Close()

	if _, err := httpGet(context.Background(), server.Client(), server.URL, 1024); err == nil {
		t.Fatal("404 should fail")
	}
	if attempts.Load() != 1 {
		t.Fatalf("attempts = %d, want 1", attempts.Load())
	}
}

func TestHTTPGetStopsAfterBoundedServerRetries(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		http.Error(w, "temporary upstream failure", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	if _, err := httpGet(context.Background(), server.Client(), server.URL, 1024); err == nil {
		t.Fatal("persistent 503 should fail")
	}
	if attempts.Load() != 3 {
		t.Fatalf("attempts = %d, want 3", attempts.Load())
	}
}

func releaseServer(t *testing.T, artifactName, checksumName string, artifact, checksums []byte) (*httptest.Server, func(context.Context) (update.Status, error)) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/artifact":
			_, _ = w.Write(artifact)
		case "/checksums":
			_, _ = w.Write(checksums)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	latest := func(context.Context) (update.Status, error) {
		return update.Status{
			LatestVersion: "v0.3.0",
			Assets: []update.Asset{
				{Name: artifactName, URL: server.URL + "/artifact"},
				{Name: checksumName, URL: server.URL + "/checksums"},
			},
		}, nil
	}
	return server, latest
}

func TestDownloadNPMUpdatePackageExtractsInstaller(t *testing.T) {
	version := "0.4.0"
	artifactName, checksumName, err := releaseAssetNames("windows", "amd64", version)
	if err != nil {
		t.Fatal(err)
	}
	artifact := []byte("setup-installer")
	checksums := checksumFile(artifactName, artifact)
	tarball := updatePackageTarball(t, map[string][]byte{
		artifactName: artifact,
		checksumName: checksums,
	})
	server := npmUpdateServer(t, version, tarball, sha512Integrity(tarball))
	m := New(Options{
		CurrentVersion: "v0.3.1", Registries: []string{server.URL}, Client: server.Client(),
		Logger: discardLogger(), GOOS: "windows", GOARCH: "amd64",
	})
	got, err := m.downloadNPMFrom(context.Background(), server.URL, version)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, artifact) {
		t.Fatalf("artifact = %q", got)
	}
}

func TestDownloadNPMUpdatePackageRejectsIntegrityAndMissingAsset(t *testing.T) {
	version := "0.4.0"
	artifactName, checksumName, _ := releaseAssetNames("windows", "amd64", version)
	artifact := []byte("setup-installer")
	tarball := updatePackageTarball(t, map[string][]byte{
		artifactName: artifact,
		checksumName: checksumFile(artifactName, artifact),
	})

	bad := npmUpdateServer(t, version, tarball, "sha512-AAAA")
	m := New(Options{
		CurrentVersion: "v0.3.1", Registries: []string{bad.URL}, Client: bad.Client(),
		Logger: discardLogger(), GOOS: "windows", GOARCH: "amd64",
	})
	if _, err := m.downloadNPMFrom(context.Background(), bad.URL, version); err == nil ||
		!strings.Contains(err.Error(), "integrity") {
		t.Fatalf("integrity error = %v", err)
	}

	missingTarball := updatePackageTarball(t, map[string][]byte{
		"unrelated": []byte("x"),
	})
	missing := npmUpdateServer(t, version, missingTarball, sha512Integrity(missingTarball))
	m.opt.Client = missing.Client()
	if _, err := m.downloadNPMFrom(context.Background(), missing.URL, version); err == nil ||
		!strings.Contains(err.Error(), "缺少") {
		t.Fatalf("missing asset error = %v", err)
	}
}

func TestDownloadNPMChainFallsBackToSecondRegistry(t *testing.T) {
	version := "0.4.0"
	artifactName, checksumName, _ := releaseAssetNames("linux", "amd64", version)
	artifact := []byte("linux-tar")
	tarball := updatePackageTarball(t, map[string][]byte{
		artifactName: artifact,
		checksumName: checksumFile(artifactName, artifact),
	})
	good := npmUpdateServer(t, version, tarball, sha512Integrity(tarball))
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "mirror down", http.StatusBadGateway)
	}))
	defer bad.Close()
	m := New(Options{
		CurrentVersion: "v0.3.1", Registries: []string{bad.URL, good.URL}, Client: good.Client(),
		Logger: discardLogger(), GOOS: "linux", GOARCH: "amd64",
	})
	got, source, err := m.downloadNPMChain(context.Background(), version)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, artifact) || source != "npm:"+npmregistry.Host(good.URL) {
		t.Fatalf("artifact=%q source=%q", got, source)
	}
}

func TestDownloadAutoFallsBackToGitHubRelease(t *testing.T) {
	version := "0.4.0"
	artifactName, checksumName, _ := releaseAssetNames("windows", "amd64", version)
	artifact := []byte("github-setup")
	githubServer, latest := releaseServer(t, artifactName, checksumName, artifact, checksumFile(artifactName, artifact))
	latestV040 := func(ctx context.Context) (update.Status, error) {
		status, err := latest(ctx)
		status.LatestVersion = "v0.4.0"
		return status, err
	}
	npmDown := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "npm down", http.StatusBadGateway)
	}))
	defer npmDown.Close()
	m := New(Options{
		CurrentVersion: "v0.3.1", Registries: []string{npmDown.URL}, Client: githubServer.Client(),
		Latest: latestV040, Logger: discardLogger(), GOOS: "windows", GOARCH: "amd64",
	})
	got, source, err := m.download(context.Background(), version, "auto")
	if err != nil {
		t.Fatal(err)
	}
	if source != "github" || !bytes.Equal(got, artifact) {
		t.Fatalf("source=%q artifact=%q", source, got)
	}
}

func TestReleaseAssetNamesAreVersionedAndPlatformSpecific(t *testing.T) {
	tests := []struct {
		goos, goarch string
		artifact     string
	}{
		{"windows", "amd64", "evergate_0.3.0_windows_amd64_setup.exe"},
		{"darwin", "arm64", "evergate_0.3.0_macos_arm64_setup.dmg"},
		{"linux", "amd64", "evergate_0.3.0_linux_amd64.tar.gz"},
	}
	for _, tt := range tests {
		artifact, checksums, err := releaseAssetNames(tt.goos, tt.goarch, "v0.3.0")
		if err != nil {
			t.Fatal(err)
		}
		if artifact != tt.artifact || checksums != "evergate_0.3.0_checksums.txt" {
			t.Fatalf("%s/%s names = %q %q", tt.goos, tt.goarch, artifact, checksums)
		}
	}
	artifact, _, err := releaseAssetNames("darwin", "arm64", "v0.3.0-rc.1")
	if err != nil || artifact != "evergate_0.3.0-rc.1_macos_arm64_setup.dmg" {
		t.Fatalf("prerelease artifact=%q err=%v", artifact, err)
	}
}

func TestDownloadRequiresExactArtifactAndChecksum(t *testing.T) {
	artifact := []byte("installer")
	name := "evergate_0.3.0_windows_amd64_setup.exe"
	checksumName := "evergate_0.3.0_checksums.txt"
	server, latest := releaseServer(t, name, checksumName, artifact, checksumFile(name, artifact))
	m := New(Options{
		CurrentVersion: "v0.2.0", Client: server.Client(), Latest: latest,
		Logger: discardLogger(), GOOS: "windows", GOARCH: "amd64",
	})
	got, source, err := m.download(context.Background(), "0.3.0", "github")
	if err != nil {
		t.Fatal(err)
	}
	if source != "github" || !bytes.Equal(got, artifact) {
		t.Fatalf("source=%q artifact=%q", source, got)
	}

	m.opt.Latest = func(context.Context) (update.Status, error) {
		status, _ := latest(context.Background())
		status.Assets = append(status.Assets, update.Asset{Name: name, URL: server.URL + "/artifact"})
		return status, nil
	}
	if _, _, err := m.download(context.Background(), "0.3.0", "github"); err == nil || !strings.Contains(err.Error(), "精确出现一次") {
		t.Fatalf("duplicate asset error = %v", err)
	}
}

func TestDownloadRejectsLegacyAssetWrongChecksumAndVersion(t *testing.T) {
	artifact := []byte("installer")
	name := "evergate_0.3.0_windows_amd64_setup.exe"
	checksumName := "evergate_0.3.0_checksums.txt"
	server, latest := releaseServer(t, name, checksumName, artifact,
		[]byte("0000000000000000000000000000000000000000000000000000000000000000  "+name+"\n"))
	m := New(Options{
		CurrentVersion: "v0.2.0", Client: server.Client(), Latest: latest,
		Logger: discardLogger(), GOOS: "windows", GOARCH: "amd64",
	})
	if _, _, err := m.download(context.Background(), "0.3.0", "github"); err == nil || !strings.Contains(err.Error(), "SHA256") {
		t.Fatalf("checksum error = %v", err)
	}

	legacyDefault := m.opt.Latest
	m.opt.Latest = func(context.Context) (update.Status, error) {
		return update.Status{LatestVersion: "v0.3.0", Assets: []update.Asset{
			{Name: "evergate-windows-amd64.exe", URL: server.URL + "/artifact"},
		}}, nil
	}
	if _, _, err := m.download(context.Background(), "0.3.0", "github"); err == nil || !strings.Contains(err.Error(), name) {
		t.Fatalf("legacy asset error = %v", err)
	}

	m.opt.Latest = func(context.Context) (update.Status, error) {
		status, _ := legacyDefault(context.Background())
		status.LatestVersion = "v0.3.1"
		return status, nil
	}
	if _, _, err := m.download(context.Background(), "0.3.0", "github"); err == nil || !strings.Contains(err.Error(), "不一致") {
		t.Fatalf("version mismatch error = %v", err)
	}
}

func TestLinuxArchiveExtractionIsStrict(t *testing.T) {
	archive := buildTarball(t, map[string][]byte{
		"README.md": []byte("readme"),
		"evergate":  []byte("new-binary"),
	})
	got, err := extractLinuxBinary(archive)
	if err != nil || string(got) != "new-binary" {
		t.Fatalf("binary=%q err=%v", got, err)
	}
	for name, archive := range map[string][]byte{
		"missing":   buildTarball(t, map[string][]byte{"bin/evergate": []byte("x")}),
		"traversal": buildTarball(t, map[string][]byte{"../evergate": []byte("x")}),
	} {
		if _, err := extractLinuxBinary(archive); err == nil {
			t.Fatalf("%s archive should be rejected", name)
		}
	}
}

func TestInstallWindowsAndMacReturnsInstallerLaunch(t *testing.T) {
	m := New(Options{CurrentVersion: "v0.2.0", Logger: discardLogger(), GOOS: "windows", GOARCH: "amd64"})
	spec, backup, err := m.installArtifact(context.Background(), "0.3.0", []byte("setup"))
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(spec.Path)
	if backup != "" || !strings.HasSuffix(spec.Path, ".exe") || !strings.Contains(strings.Join(spec.Args, " "), "/NORESTART") {
		t.Fatalf("windows spec=%#v backup=%q", spec, backup)
	}

	m.opt.GOOS = "darwin"
	m.opt.GOARCH = "arm64"
	spec, backup, err = m.installArtifact(context.Background(), "0.3.0", []byte("dmg"))
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(spec.Args[0])
	if backup != "" || spec.Path != "/usr/bin/open" || !strings.HasSuffix(spec.Args[0], ".dmg") {
		t.Fatalf("darwin spec=%#v backup=%q", spec, backup)
	}
}

func TestMacVerificationChecksNotarizationAndSignature(t *testing.T) {
	previous := runPlatformCommand
	var calls [][]string
	runPlatformCommand = func(_ context.Context, name string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{name}, args...))
		return []byte("ok"), nil
	}
	t.Cleanup(func() { runPlatformCommand = previous })
	m := New(Options{CurrentVersion: "v0.2.0", Logger: discardLogger(), GOOS: "darwin", GOARCH: "arm64"})
	if err := m.verifyArtifact(context.Background(), []byte("dmg")); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || calls[0][0] != "/usr/bin/xcrun" || calls[1][0] != "/usr/sbin/spctl" {
		t.Fatalf("verification calls = %#v", calls)
	}

	runPlatformCommand = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return []byte("invalid signature"), fmt.Errorf("exit status 1")
	}
	if err := m.verifyArtifact(context.Background(), []byte("dmg")); err == nil ||
		!strings.Contains(err.Error(), "校验失败") {
		t.Fatalf("signature failure error = %v", err)
	}
}

func TestInstallLinuxReplacesBinaryAndKeepsBackup(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "evergate")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	archive := buildTarball(t, map[string][]byte{"evergate": []byte("new")})
	m := New(Options{
		CurrentVersion: "v0.2.0", Executable: exe, WorkDir: dir,
		Logger: discardLogger(), GOOS: "linux", GOARCH: "amd64",
	})
	spec, backup, err := m.installArtifact(context.Background(), "0.3.0", archive)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(exe)
	old, _ := os.ReadFile(backup)
	if string(got) != "new" || string(old) != "old" || spec.Path != exe {
		t.Fatalf("binary=%q backup=%q spec=%#v", got, old, spec)
	}
}

func TestStartLinuxDownloadsVerifiesReplacesAndRequestsRestart(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "evergate")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	archive := buildTarball(t, map[string][]byte{"evergate": []byte("new")})
	name := "evergate_0.3.0_linux_amd64.tar.gz"
	checksumName := "evergate_0.3.0_checksums.txt"
	server, latest := releaseServer(t, name, checksumName, archive, checksumFile(name, archive))
	restarted := make(chan struct{}, 1)
	m := New(Options{
		CurrentVersion: "v0.2.0", Executable: exe, WorkDir: dir,
		Client: server.Client(), Latest: latest, Logger: discardLogger(),
		GOOS: "linux", GOARCH: "amd64", RequestRestart: func() { restarted <- struct{}{} },
	})
	if _, err := m.Start("0.3.0", "github"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-restarted:
	case <-time.After(5 * time.Second):
		t.Fatal("restart not requested")
	}
	got, _ := os.ReadFile(exe)
	job := m.Snapshot()
	if string(got) != "new" || job == nil || job.Stage != StageRestarting || job.BackupPath == "" || job.Source != "github" {
		t.Fatalf("binary=%q job=%#v", got, job)
	}
}

func TestCapabilityUsesStandardInstallLocations(t *testing.T) {
	root := t.TempDir()
	windowsExe := filepath.Join(root, "Programs", "EverGate", "evergate.exe")
	if err := os.MkdirAll(filepath.Dir(windowsExe), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(windowsExe, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := New(Options{CurrentVersion: "v0.2.0", Executable: windowsExe, Logger: discardLogger(), GOOS: "windows", GOARCH: "amd64"})
	if cap := m.Capability(); !cap.Available {
		t.Fatalf("standard Windows capability = %#v", cap)
	}
	npmExe := filepath.Join(root, "node_modules", "@heguoliang", "evergate", "evergate.exe")
	if err := os.MkdirAll(filepath.Dir(npmExe), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(npmExe, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	m.opt.Executable = npmExe
	if cap := m.Capability(); cap.Available || !strings.Contains(cap.Reason, "标准") {
		t.Fatalf("npm Windows capability = %#v", cap)
	}

	for _, version := range []string{"dev", "v0.2.0-dirty", "v0.2.0-9-gabcdef0"} {
		m := New(Options{CurrentVersion: version, Executable: windowsExe, Logger: discardLogger(), GOOS: "windows", GOARCH: "amd64"})
		if cap := m.Capability(); cap.Available || !cap.Dev {
			t.Fatalf("development capability = %#v", cap)
		}
	}
}

func TestCancelStopsDownloadBeforeInstall(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/checksums" {
			close(started)
			<-r.Context().Done()
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	name := "evergate_0.3.0_linux_amd64.tar.gz"
	latest := func(context.Context) (update.Status, error) {
		return update.Status{LatestVersion: "v0.3.0", Assets: []update.Asset{
			{Name: name, URL: server.URL + "/artifact"},
			{Name: "evergate_0.3.0_checksums.txt", URL: server.URL + "/checksums"},
		}}, nil
	}
	dir := t.TempDir()
	exe := filepath.Join(dir, "evergate")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := New(Options{
		CurrentVersion: "v0.2.0", Executable: exe, WorkDir: dir, Client: server.Client(),
		Latest: latest, Logger: discardLogger(), GOOS: "linux", GOARCH: "amd64",
		RequestRestart: func() { t.Error("canceled update must not restart") },
	})
	if _, err := m.Start("0.3.0", "github"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("download did not start")
	}
	if err := m.Cancel(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		job := m.Snapshot()
		if job != nil && job.Stage == StageFailed {
			if !job.Cancelable {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("job after cancel = %#v", m.Snapshot())
}

func TestReleaseChannelRejectsUnknownAndOlderVersions(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "evergate")
	_ = os.WriteFile(exe, []byte("x"), 0o755)
	m := New(Options{
		CurrentVersion: "v0.2.0", Executable: exe, Logger: discardLogger(),
		GOOS: "linux", GOARCH: "amd64", RequestRestart: func() {},
	})
	if _, err := m.Start("0.3.0", "ftp"); err == nil {
		t.Fatal("unknown channel must be rejected")
	}
	if _, err := m.Start("0.2.0", "auto"); err == nil {
		t.Fatal("same version must be rejected")
	}
}
