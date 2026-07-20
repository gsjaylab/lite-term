package auth

import (
	"encoding/base64"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTokenIsBoundToSubject(t *testing.T) {
	store := NewStore(time.Minute, time.Now)
	token, err := store.Issue("1000")
	if err != nil {
		t.Fatal(err)
	}
	if store.Consume(token, "1001") {
		t.Fatal("token accepted for a different subject")
	}
}

func TestTokenIssuanceIsRateLimitedPerSubject(t *testing.T) {
	now := time.Unix(100, 0)
	store := NewStore(time.Minute, func() time.Time { return now })
	if _, err := store.Issue("1000"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Issue("1000"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("second issue error=%v, want ErrRateLimited", err)
	}
	if _, err := store.Issue("1001"); err != nil {
		t.Fatalf("different subject was limited: %v", err)
	}
}

func TestIssuedTokenUses32BytesAndRawURLAlphabet(t *testing.T) {
	store := NewStore(time.Minute, time.Now)
	token, err := store.Issue("1000")
	if err != nil {
		t.Fatal(err)
	}
	if len(token) != 43 {
		t.Fatalf("token length=%d, want 43", len(token))
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Fatalf("token is not raw URL base64: %v", err)
	}
	if len(raw) != 32 {
		t.Fatalf("decoded token length=%d, want 32", len(raw))
	}
}

func TestConcurrentTokenConsumptionHasExactlyOneWinner(t *testing.T) {
	store := NewStore(time.Minute, time.Now)
	token, err := store.Issue("1000")
	if err != nil {
		t.Fatal(err)
	}
	const consumers = 64
	start := make(chan struct{})
	var winners atomic.Int32
	var wg sync.WaitGroup
	for range consumers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if store.Consume(token, "1000") {
				winners.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()
	if got := winners.Load(); got != 1 {
		t.Fatalf("successful consumers=%d, want 1", got)
	}
}

func TestTokenCanBeConsumedOnlyOnce(t *testing.T) {
	now := time.Unix(100, 0)
	store := NewStore(time.Minute, func() time.Time { return now })
	token, err := store.Issue("1000")
	if err != nil {
		t.Fatal(err)
	}
	if !store.Consume(token, "1000") {
		t.Fatal("first consume rejected")
	}
	if store.Consume(token, "1000") {
		t.Fatal("token reused")
	}
}

func TestExpiredAndUnknownTokensAreRejected(t *testing.T) {
	now := time.Unix(100, 0)
	store := NewStore(time.Minute, func() time.Time { return now })
	token, err := store.Issue("1000")
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute + time.Nanosecond)
	if store.Consume(token, "1000") {
		t.Fatal("expired token accepted")
	}
	if store.Consume("not-a-token", "1000") {
		t.Fatal("unknown token accepted")
	}
}

func TestExpiredTokensArePurged(t *testing.T) {
	now := time.Unix(100, 0)
	store := NewStore(time.Minute, func() time.Time { return now })
	if _, err := store.Issue("1000"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute + time.Nanosecond)
	if _, err := store.Issue("1000"); err != nil {
		t.Fatal(err)
	}
	if got := len(store.grants); got != 1 {
		t.Fatalf("stored tokens=%d, want 1", got)
	}
}
