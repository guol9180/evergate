package relay

import (
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"gorm.io/gorm"

	"evergate/internal/store"
)

// LogWriter 请求日志异步批量落库：缓冲 channel + 定时/定量刷盘，
// 绝不阻塞请求路径（任务书 4.8：带缓冲 channel）。
type LogWriter struct {
	db      *gorm.DB
	ch      chan *store.RequestLog
	logger  *slog.Logger
	dropped atomic.Int64
	wg      sync.WaitGroup
	stop    chan struct{}
	stopped sync.Once
}

// NewLogWriter bufSize 为缓冲容量；启动后台刷盘协程。
func NewLogWriter(db *gorm.DB, bufSize int, logger *slog.Logger) *LogWriter {
	if bufSize <= 0 {
		bufSize = 256
	}
	if logger == nil {
		logger = slog.Default()
	}
	w := &LogWriter{
		db: db, ch: make(chan *store.RequestLog, bufSize),
		logger: logger, stop: make(chan struct{}),
	}
	w.wg.Add(1)
	go w.loop()
	return w
}

// Submit 非阻塞提交一条请求日志；缓冲满则丢弃并计数（保请求路径不受日志拖累）。
func (w *LogWriter) Submit(log *store.RequestLog) {
	if log == nil {
		return
	}
	select {
	case w.ch <- log:
	default:
		n := w.dropped.Add(1)
		if n%100 == 1 { // 周期性告警，避免刷屏
			w.logger.Warn("request log dropped due to full buffer", slog.Int64("dropped_total", n))
		}
	}
}

// Close 停止后台协程并刷出剩余日志。
func (w *LogWriter) Close() {
	w.stopped.Do(func() { close(w.stop) })
	w.wg.Wait()
}

func (w *LogWriter) loop() {
	defer w.wg.Done()
	batch := make([]*store.RequestLog, 0, 64)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := w.db.Create(&batch).Error; err != nil {
			w.logger.Error("flush request logs failed", slog.Any("error", err), slog.Int("count", len(batch)))
		}
		batch = batch[:0]
	}
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case log := <-w.ch:
			batch = append(batch, log)
			if len(batch) >= 64 {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-w.stop:
			for {
				select {
				case log := <-w.ch:
					batch = append(batch, log)
					if len(batch) >= 64 {
						flush()
					}
				default:
					flush()
					return
				}
			}
		}
	}
}
