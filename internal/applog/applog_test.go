package applog

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotatingFileKeepsOneBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "evergate.log")
	writer, err := newRotatingFile(path, 8, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("abcdefgh")); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("ij")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	backup, err := os.ReadFile(path + ".1")
	if err != nil {
		t.Fatal(err)
	}
	if string(current) != "ij" || string(backup) != "abcdefgh" {
		t.Fatalf("current=%q backup=%q", current, backup)
	}
}

func TestFanoutContinuesAfterWriterError(t *testing.T) {
	var good strings.Builder
	writer := fanoutWriter{writers: []io.Writer{
		writerFunc(func([]byte) (int, error) { return 0, os.ErrClosed }),
		writerFunc(func(p []byte) (int, error) { return good.Write(p) }),
	}}
	if n, err := writer.Write([]byte("still-written")); err != nil || n != len("still-written") {
		t.Fatalf("write n=%d err=%v", n, err)
	}
	if good.String() != "still-written" {
		t.Fatalf("good writer = %q", good.String())
	}
}

func TestOpenCreatesLogDirectoryAndJSONLog(t *testing.T) {
	dataDir := t.TempDir()
	logger, path, closer, err := Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("startup", "component", "test")
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dataDir, "logs", "evergate.log"); path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"msg":"startup"`) || !strings.Contains(string(data), `"component":"test"`) {
		t.Fatalf("log content = %s", data)
	}
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }
