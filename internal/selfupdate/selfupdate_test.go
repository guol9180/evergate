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
	"testing"
	"time"

	"evergate/internal/update"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func buildTarball(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatalf("tar header: %v", err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatalf("tar write: %v", err)
	}
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}

func sha512Integrity(data []byte) string {
	sum := sha512.Sum512(data)
	return "sha512-" + base64.StdEncoding.EncodeToString(sum[:])
}

func npmServer(t *testing.T, tarball []byte, integrity string) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/@heguoliang/evergate-win32-x64/0.1.25":
			_, _ = fmt.Fprintf(w, `{"dist":{"tarball":%q,"integrity":%q}}`, srv.URL+"/pkg.tgz", integrity)
		case r.URL.Path == "/pkg.tgz":
			_, _ = w.Write(tarball)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestDownloadNPMVerifiesIntegrityAndExtractsBinary(t *testing.T) {
	content := []byte("new-binary-content")
	tarball := buildTarball(t, "package/bin/evergate.exe", content)
	srv := npmServer(t, tarball, sha512Integrity(tarball))

	m := New(Options{
		CurrentVersion: "v0.1.24", Registry: srv.URL, Client: srv.Client(),
		Logger: discardLogger(), GOOS: "windows", GOARCH: "amd64",
	})
	got, err := m.downloadNPM(context.Background(), "0.1.25")
	if err != nil {
		t.Fatalf("download npm: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("binary = %q, want %q", got, content)
	}
}

func TestDownloadNPMRejectsBadIntegrity(t *testing.T) {
	tarball := buildTarball(t, "package/bin/evergate.exe", []byte("content"))
	srv := npmServer(t, tarball, "sha512-AAAA")
	m := New(Options{
		CurrentVersion: "v0.1.24", Registry: srv.URL, Client: srv.Client(),
		Logger: discardLogger(), GOOS: "windows", GOARCH: "amd64",
	})
	if _, err := m.downloadNPM(context.Background(), "0.1.25"); err == nil || !strings.Contains(err.Error(), "integrity") {
		t.Fatalf("err = %v, want integrity failure", err)
	}
}

func TestDownloadGitHubVerifiesChecksum(t *testing.T) {
	binary := []byte("github-binary")
	sum := sha256.Sum256(binary)
	checksums := fmt.Sprintf("%s  release/evergate-windows-amd64.exe\n", hex.EncodeToString(sum[:]))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bin":
			_, _ = w.Write(binary)
		case "/checksums":
			_, _ = w.Write([]byte(checksums))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	m := New(Options{
		CurrentVersion: "v0.1.24", Client: srv.Client(), Logger: discardLogger(),
		GOOS: "windows", GOARCH: "amd64",
		Latest: func(context.Context) (update.Status, error) {
			return update.Status{
				LatestVersion: "v0.1.25",
				Assets: []update.Asset{
					{Name: "evergate-windows-amd64.exe", URL: srv.URL + "/bin"},
					{Name: "checksums.txt", URL: srv.URL + "/checksums"},
				},
			}, nil
		},
	})
	got, err := m.downloadGitHub(context.Background(), "0.1.25")
	if err != nil {
		t.Fatalf("download github: %v", err)
	}
	if !bytes.Equal(got, binary) {
		t.Fatalf("binary mismatch")
	}

	m.opt.Latest = func(context.Context) (update.Status, error) {
		return update.Status{
			LatestVersion: "v0.1.25",
			Assets: []update.Asset{
				{Name: "evergate-windows-amd64.exe", URL: srv.URL + "/bin"},
				{Name: "checksums.txt", URL: srv.URL + "/checksums-bad"},
			},
		}, nil
	}
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("0000000000000000000000000000000000000000000000000000000000000000  release/evergate-windows-amd64.exe\n"))
	}))
	defer bad.Close()
	m.opt.Client = bad.Client()
	m.opt.Latest = func(context.Context) (update.Status, error) {
		return update.Status{
			LatestVersion: "v0.1.25",
			Assets: []update.Asset{
				{Name: "evergate-windows-amd64.exe", URL: srv.URL + "/bin"},
				{Name: "checksums.txt", URL: bad.URL},
			},
		}, nil
	}
	if _, err := m.downloadGitHub(context.Background(), "0.1.25"); err == nil || !strings.Contains(err.Error(), "SHA256") {
		t.Fatalf("err = %v, want checksum failure", err)
	}
}

func TestReplaceExecutableKeepsBackup(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "evergate.exe")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	backup, err := replaceExecutable(exe, "0.1.25", []byte("new"))
	if err != nil {
		t.Fatalf("replace: %v", err)
	}
	got, _ := os.ReadFile(exe)
	if string(got) != "new" {
		t.Fatalf("exe = %q", got)
	}
	old, err := os.ReadFile(backup)
	if err != nil || string(old) != "old" {
		t.Fatalf("backup = %q err=%v", old, err)
	}
}

func TestStartSuccessReplacesAndRequestsRestart(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "evergate.exe")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	content := []byte("new-binary")
	tarball := buildTarball(t, "package/bin/evergate.exe", content)
	srv := npmServer(t, tarball, sha512Integrity(tarball))
	restarted := make(chan struct{}, 1)
	m := New(Options{
		CurrentVersion: "v0.1.24", Executable: exe, Args: []string{"-config", "x"}, WorkDir: dir,
		Registry: srv.URL, Client: srv.Client(), Logger: discardLogger(), GOOS: "windows", GOARCH: "amd64",
		RequestRestart: func() { restarted <- struct{}{} },
	})
	if _, err := m.Start("0.1.25", "npm"); err != nil {
		t.Fatalf("start: %v", err)
	}
	select {
	case <-restarted:
	case <-time.After(5 * time.Second):
		t.Fatal("restart not requested")
	}
	got, _ := os.ReadFile(exe)
	if !bytes.Equal(got, content) {
		t.Fatalf("exe = %q", got)
	}
	job := m.Snapshot()
	if job == nil || job.Stage != StageRestarting || job.BackupPath == "" {
		t.Fatalf("job = %#v", job)
	}
	if _, err := os.Stat(job.BackupPath); err != nil {
		t.Fatalf("backup missing: %v", err)
	}
	spec, ok := m.PendingLaunch()
	if !ok || spec.Path != exe || len(spec.Args) != 2 {
		t.Fatalf("pending = %#v ok=%v", spec, ok)
	}
}

func TestStartIntegrityFailureKeepsOldBinary(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "evergate.exe")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	tarball := buildTarball(t, "package/bin/evergate.exe", []byte("new"))
	srv := npmServer(t, tarball, "sha512-wrong")
	restarted := make(chan struct{}, 1)
	m := New(Options{
		CurrentVersion: "v0.1.24", Executable: exe, WorkDir: dir,
		Registry: srv.URL, Client: srv.Client(), Logger: discardLogger(), GOOS: "windows", GOARCH: "amd64",
		RequestRestart: func() { restarted <- struct{}{} },
	})
	if _, err := m.Start("0.1.25", "npm"); err != nil {
		t.Fatalf("start: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		job := m.Snapshot()
		if job != nil && job.Stage == StageFailed {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	job := m.Snapshot()
	if job == nil || job.Stage != StageFailed || !strings.Contains(job.Error, "integrity") {
		t.Fatalf("job = %#v", job)
	}
	got, _ := os.ReadFile(exe)
	if string(got) != "old" {
		t.Fatalf("old binary was modified: %q", got)
	}
	select {
	case <-restarted:
		t.Fatal("restart should not be requested")
	default:
	}
}

func TestCapabilityRejectsDevelopmentVersion(t *testing.T) {
	m := New(Options{CurrentVersion: "dev", Executable: "x", Logger: discardLogger()})
	if cap := m.Capability(); cap.Available {
		t.Fatalf("dev capability = %#v", cap)
	}
	if _, err := m.Start("0.1.25", "npm"); err == nil {
		t.Fatal("dev version should refuse update")
	}
}

func TestStartRejectsSameOrOlderVersion(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "evergate.exe")
	_ = os.WriteFile(exe, []byte("old"), 0o755)
	m := New(Options{
		CurrentVersion: "v0.1.24", Executable: exe, WorkDir: dir,
		Logger: discardLogger(), GOOS: "windows", GOARCH: "amd64",
		RequestRestart: func() {},
	})
	if _, err := m.Start("0.1.24", "npm"); err == nil {
		t.Fatal("same version should be rejected")
	}
}

func TestCapabilityRejectsMissingInstallDirectory(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "missing", "evergate.exe")
	m := New(Options{
		CurrentVersion: "v0.1.24", Executable: exe,
		Logger: discardLogger(), GOOS: "windows", GOARCH: "amd64",
		RequestRestart: func() {},
	})
	if cap := m.Capability(); cap.Available {
		t.Fatalf("missing dir capability = %#v", cap)
	}
	if _, err := m.Start("0.1.25", "npm"); err == nil {
		t.Fatal("missing install directory should refuse update")
	}
	if _, err := replaceExecutable(exe, "0.1.25", []byte("new")); err == nil {
		t.Fatal("replace should fail for missing directory")
	}
}

func TestDownloadAutoFallsBackToGitHub(t *testing.T) {
	binary := []byte("github-binary")
	sum := sha256.Sum256(binary)
	checksums := fmt.Sprintf("%s  release/evergate-windows-amd64.exe\n", hex.EncodeToString(sum[:]))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bin":
			_, _ = w.Write(binary)
		case "/checksums":
			_, _ = w.Write([]byte(checksums))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	m := New(Options{
		CurrentVersion: "v0.1.24", Registry: srv.URL, Client: srv.Client(), Logger: discardLogger(),
		GOOS: "windows", GOARCH: "amd64",
		Latest: func(context.Context) (update.Status, error) {
			return update.Status{
				LatestVersion: "v0.1.25",
				Assets: []update.Asset{
					{Name: "evergate-windows-amd64.exe", URL: srv.URL + "/bin"},
					{Name: "checksums.txt", URL: srv.URL + "/checksums"},
				},
			}, nil
		},
	})
	got, source, err := m.download(context.Background(), "0.1.25", "auto")
	if err != nil {
		t.Fatalf("auto download: %v", err)
	}
	if source != "github" || !bytes.Equal(got, binary) {
		t.Fatalf("source=%s binary=%q", source, got)
	}
}
