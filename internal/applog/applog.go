// Package applog 提供 stdout + 文件的双写运行日志。
package applog

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

const (
	defaultMaxSize = 10 << 20
	defaultBackups = 1
)

// Open 创建 dataDir/logs/evergate.log，并返回同时写 stdout 与刷新文件的 logger。
func Open(dataDir string) (*slog.Logger, string, io.Closer, error) {
	logDir := filepath.Join(dataDir, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, "", nil, fmt.Errorf("create log directory: %w", err)
	}
	path := filepath.Join(logDir, "evergate.log")
	fileWriter, err := newRotatingFile(path, defaultMaxSize, defaultBackups)
	if err != nil {
		return nil, "", nil, err
	}
	writer := fanoutWriter{writers: []io.Writer{os.Stdout, fileWriter}}
	logger := slog.New(slog.NewJSONHandler(writer, &slog.HandlerOptions{Level: slog.LevelInfo}))
	return logger, path, fileWriter, nil
}

type fanoutWriter struct {
	writers []io.Writer
}

// Write 独立写入每个目标；GUI 构建没有有效 stdout 时不能阻断文件日志。
func (w fanoutWriter) Write(p []byte) (int, error) {
	for _, writer := range w.writers {
		_, _ = writer.Write(p)
	}
	return len(p), nil
}

type rotatingFile struct {
	mu      sync.Mutex
	path    string
	maxSize int64
	backups int
	file    *os.File
	size    int64
}

func newRotatingFile(path string, maxSize int64, backups int) (*rotatingFile, error) {
	if maxSize <= 0 {
		return nil, fmt.Errorf("max log size must be positive")
	}
	if backups < 0 {
		return nil, fmt.Errorf("log backups must be >= 0")
	}
	w := &rotatingFile{path: path, maxSize: maxSize, backups: backups}
	if err := w.open(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *rotatingFile) open() error {
	file, err := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return fmt.Errorf("stat log file: %w", err)
	}
	w.file = file
	w.size = info.Size()
	return nil
}

func (w *rotatingFile) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return 0, os.ErrClosed
	}
	if w.size > 0 && w.size+int64(len(p)) > w.maxSize {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := w.file.Write(p)
	w.size += int64(n)
	return n, err
}

func (w *rotatingFile) rotate() error {
	if err := w.file.Close(); err != nil {
		return fmt.Errorf("close log before rotate: %w", err)
	}
	w.file = nil
	if w.backups == 0 {
		if err := os.Remove(w.path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove log before rotate: %w", err)
		}
	} else {
		for i := w.backups; i >= 1; i-- {
			src := w.path
			if i > 1 {
				src = fmt.Sprintf("%s.%d", w.path, i-1)
			}
			dst := fmt.Sprintf("%s.%d", w.path, i)
			_ = os.Remove(dst)
			if err := os.Rename(src, dst); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("rotate log: %w", err)
			}
		}
	}
	w.size = 0
	return w.open()
}

func (w *rotatingFile) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}
