package relay

// 上游读取上限：异常或恶意上游只需发一个超长行、无限前导或超大响应体就能撑爆网关内存。
// 流式只限制单行、单事件与首个 data 之前的前导（不缓冲整段，SSE 红线不变）；非流式限制整体。

import (
	"bufio"
	"errors"
	"io"
)

// 上限用变量而非常量：测试把上限调小，不必真的构造 64MB 响应体。
var (
	maxSSELineBytes     = 1 << 20 // 单行 SSE（含 data 行）
	maxSSEEventBytes    = 1 << 20 // 单个事件：event / data 多行累计
	maxSSEPreambleBytes = 1 << 20 // 首个 data 之前的注释、空行与无数据事件累计
	// 非流式响应体；int64 是 io.LimitReader 的参数类型。
	maxNonStreamBodyBytes int64 = 64 << 20
)

var (
	errSSELineTooLarge      = errors.New("upstream sse line exceeds 1MB")
	errSSEEventTooLarge     = errors.New("upstream sse event exceeds 1MB")
	errSSEPreambleTooLarge  = errors.New("upstream sse preamble exceeds 1MB before first data")
	errUpstreamBodyTooLarge = errors.New("upstream response body exceeds 64MB")
)

// sseLineReader 按行读取上游 SSE；超过 maxSSELineBytes 的行直接报错，不无限累积。
type sseLineReader struct{ br *bufio.Reader }

func newSSELineReader(r io.Reader) *sseLineReader {
	return &sseLineReader{br: bufio.NewReaderSize(r, 32*1024)}
}

// readLine 读到下一个 '\n'（含换行）；EOF 时返回剩余字节与 io.EOF，语义同 bufio.Reader.ReadBytes。
func (l *sseLineReader) readLine() ([]byte, error) {
	var line []byte
	for {
		chunk, err := l.br.ReadSlice('\n')
		if len(line)+len(chunk) > maxSSELineBytes {
			return nil, errSSELineTooLarge
		}
		line = append(line, chunk...)
		switch {
		case err == nil:
			return line, nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		default:
			return line, err
		}
	}
}

// readUpstreamBody 读完非流式响应体；超过 maxNonStreamBodyBytes 按上游失败处理。
func readUpstreamBody(body io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, maxNonStreamBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxNonStreamBodyBytes {
		return nil, errUpstreamBodyTooLarge
	}
	return data, nil
}
