package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"sync"
	"time"
)

var ErrRateLimited = errors.New("token issuance rate limited")

const maxSubjects = 1024

type grant struct {
	subject string
	expires time.Time
}

type Store struct {
	mu     sync.Mutex
	ttl    time.Duration
	now    func() time.Time
	grants map[string]grant
	issued map[string]time.Time
}

// NewStore 创建一个内存中的一次性授权仓库。仓库只保存 token 摘要所需的
// 最小上下文，不保存密码、Cookie 或其他 fnOS 会话信息。
func NewStore(ttl time.Duration, now func() time.Time) *Store {
	return &Store{
		ttl:    ttl,
		now:    now,
		grants: make(map[string]grant),
		issued: make(map[string]time.Time),
	}
}

// Issue 为指定 fnOS UID 签发一次性 token。同一 UID 每秒最多签发一次，
// 且新 token 会撤销该 UID 尚未使用的旧 token，避免重试造成授权堆积。
func (s *Store) Issue(subject string) (string, error) {
	if subject == "" {
		return "", ErrRateLimited
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)

	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	s.deleteExpired(now)
	if last, ok := s.issued[subject]; ok && now.Sub(last) < time.Second {
		return "", ErrRateLimited
	}
	if _, known := s.issued[subject]; !known && len(s.issued) >= maxSubjects {
		return "", ErrRateLimited
	}
	// 每个主体最多保留一个有效授权：既限制内存占用，也让重试自动撤销
	// 可能已经泄露但尚未使用的旧 token。
	for value, existing := range s.grants {
		if existing.subject == subject {
			delete(s.grants, value)
		}
	}
	s.grants[token] = grant{subject: subject, expires: now.Add(s.ttl)}
	s.issued[subject] = now

	return token, nil
}

// Consume 原子地验证并销毁 token。即使 UID 不匹配也会销毁该 token，
// 从而避免被截获的 token 反复探测其他用户身份。
func (s *Store) Consume(token, subject string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	value, ok := s.grants[token]
	s.deleteExpired(now)
	delete(s.grants, token)
	return ok && value.subject == subject && !now.After(value.expires)
}

func (s *Store) deleteExpired(now time.Time) {
	// grants 和 issued 分开清理：token 可能已经被消费，但签发频率记录仍需
	// 保留到限流窗口结束；最终以 TTL 作为最迟回收时间。
	for token, value := range s.grants {
		if now.After(value.expires) {
			delete(s.grants, token)
			delete(s.issued, value.subject)
		}
	}
	for subject, last := range s.issued {
		if now.Sub(last) >= s.ttl {
			delete(s.issued, subject)
		}
	}
}
