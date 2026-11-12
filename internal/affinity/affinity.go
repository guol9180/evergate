// Package affinity 提供进程内会话亲和表（任务书 4.5① 会话亲和）：
// 同一下游令牌 + 模型 + 会话 ID 在 TTL 内优先复用最近成功的渠道与 key。
//
// 安全与容量约定：
//   - 键只保存 token/model/session 的 SHA-256 指纹，绝不保存 X-Session-Id 原文；
//   - 空会话、超长会话、TTL 过期与容量淘汰一律视为未命中，由调用方回退普通调度；
//   - 仅进程内存，不持久化、不跨实例；重启后亲和自然丢失并重建。
package affinity

import (
	"container/list"
	"crypto/sha256"
	"encoding/binary"
	"hash"
	"io"
	"sync"
	"time"
)

const (
	// DefaultTTL 粘性记录有效期（任务书：TTL 10 分钟）。
	DefaultTTL = 10 * time.Minute
	// DefaultCapacity 粘性表最大条目数，超出按 LRU 淘汰。
	DefaultCapacity = 4096
	// MaxSessionLen 允许的 X-Session-Id 最大字节数，超长直接视为未携带。
	MaxSessionLen = 128
)

// Entry 是粘性记录命中结果：上次成功的渠道与 key。
type Entry struct {
	ChannelID int64
	KeyID     int64
}

type record struct {
	fingerprint [sha256.Size]byte
	channelID   int64
	keyID       int64
	expiresAt   time.Time
}

// Table 是并发安全的有界 LRU 亲和表。
type Table struct {
	mu       sync.Mutex
	entries  map[[sha256.Size]byte]*list.Element
	lru      *list.List // 头部最新，尾部最旧
	capacity int
	ttl      time.Duration
	now      func() time.Time
}

// New 创建亲和表；capacity/ttl/now 为零值时使用默认值。
func New(capacity int, ttl time.Duration, now func() time.Time) *Table {
	if capacity <= 0 {
		capacity = DefaultCapacity
	}
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	if now == nil {
		now = time.Now
	}
	return &Table{
		entries:  make(map[[sha256.Size]byte]*list.Element, capacity),
		lru:      list.New(),
		capacity: capacity,
		ttl:      ttl,
		now:      now,
	}
}

// NormalizeSession 清洗请求头值：去首尾空白；空或超长返回 ""（调用方回退普通调度）。
func NormalizeSession(raw string) string {
	session := raw
	// 只裁剪 ASCII 空白，避免改变非 ASCII 会话 ID 语义。
	for len(session) > 0 && (session[0] == ' ' || session[0] == '\t') {
		session = session[1:]
	}
	for len(session) > 0 {
		last := session[len(session)-1]
		if last != ' ' && last != '\t' {
			break
		}
		session = session[:len(session)-1]
	}
	if session == "" || len(session) > MaxSessionLen {
		return ""
	}
	return session
}

// Lookup 返回未过期的粘性记录；命中即刷新 LRU。
func (t *Table) Lookup(tokenID int64, model, session string) (Entry, bool) {
	if t == nil || tokenID == 0 || session == "" {
		return Entry{}, false
	}
	key := fingerprint(tokenID, model, session)
	t.mu.Lock()
	defer t.mu.Unlock()
	el, ok := t.entries[key]
	if !ok {
		return Entry{}, false
	}
	rec := el.Value.(*record)
	if !t.now().Before(rec.expiresAt) {
		t.removeLocked(el)
		return Entry{}, false
	}
	t.lru.MoveToFront(el)
	return Entry{ChannelID: rec.channelID, KeyID: rec.keyID}, true
}

// Remember 记录（或刷新）一次成功命中。
func (t *Table) Remember(tokenID int64, model, session string, channelID, keyID int64) {
	if t == nil || tokenID == 0 || session == "" || channelID == 0 || keyID == 0 {
		return
	}
	key := fingerprint(tokenID, model, session)
	expires := t.now().Add(t.ttl)
	t.mu.Lock()
	defer t.mu.Unlock()
	if el, ok := t.entries[key]; ok {
		rec := el.Value.(*record)
		rec.channelID, rec.keyID, rec.expiresAt = channelID, keyID, expires
		t.lru.MoveToFront(el)
		return
	}
	el := t.lru.PushFront(&record{
		fingerprint: key,
		channelID:   channelID,
		keyID:       keyID,
		expiresAt:   expires,
	})
	t.entries[key] = el
	for t.lru.Len() > t.capacity {
		oldest := t.lru.Back()
		if oldest == nil {
			break
		}
		t.removeLocked(oldest)
	}
}

// Forget 删除会话的粘性记录。
func (t *Table) Forget(tokenID int64, model, session string) {
	if t == nil || tokenID == 0 || session == "" {
		return
	}
	key := fingerprint(tokenID, model, session)
	t.mu.Lock()
	defer t.mu.Unlock()
	if el, ok := t.entries[key]; ok {
		t.removeLocked(el)
	}
}

// ForgetIfMatches 仅在记录仍指向给定渠道与 key 时删除，
// 避免慢失败请求误删其他并发请求刚写入的新亲和。
func (t *Table) ForgetIfMatches(tokenID int64, model, session string, channelID, keyID int64) {
	if t == nil || tokenID == 0 || session == "" {
		return
	}
	key := fingerprint(tokenID, model, session)
	t.mu.Lock()
	defer t.mu.Unlock()
	el, ok := t.entries[key]
	if !ok {
		return
	}
	rec := el.Value.(*record)
	if rec.channelID == channelID && rec.keyID == keyID {
		t.removeLocked(el)
	}
}

// Len 返回当前条目数（含尚未清理的过期记录），用于容量与诊断断言。
func (t *Table) Len() int {
	if t == nil {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.lru.Len()
}

func (t *Table) removeLocked(el *list.Element) {
	rec := el.Value.(*record)
	delete(t.entries, rec.fingerprint)
	t.lru.Remove(el)
}

// fingerprint 对 token/model/session 做长度前缀编码后 SHA-256，
// 避免拼接分隔符造成的不同三元组同键。
func fingerprint(tokenID int64, model, session string) [sha256.Size]byte {
	h := sha256.New()
	var idBuf [8]byte
	binary.BigEndian.PutUint64(idBuf[:], uint64(tokenID))
	_, _ = h.Write(idBuf[:])
	writePart(h, model)
	writePart(h, session)
	var out [sha256.Size]byte
	copy(out[:], h.Sum(nil))
	return out
}

func writePart(h hash.Hash, s string) {
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(s)))
	_, _ = h.Write(n[:])
	_, _ = io.WriteString(h, s)
}
