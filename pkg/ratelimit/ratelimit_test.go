package ratelimit

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestMemoryWindow(t *testing.T) {
	m := NewMemory()
	clock := time.Now()
	m.now = func() time.Time { return clock }
	ctx := context.Background()

	for i := 1; i <= 3; i++ {
		r, _ := m.Allow(ctx, "k", 3, time.Minute)
		if !r.Allowed || r.Remaining != 3-i {
			t.Fatalf("request %d: %+v", i, r)
		}
	}
	r, _ := m.Allow(ctx, "k", 3, time.Minute)
	if r.Allowed || r.Remaining != 0 || r.RetryAfter <= 0 || r.RetryAfter > time.Minute {
		t.Fatalf("over the limit: %+v", r)
	}
	if other, _ := m.Allow(ctx, "other", 3, time.Minute); !other.Allowed {
		t.Fatal("keys must not share a counter")
	}

	clock = clock.Add(time.Minute + time.Second)
	if r, _ := m.Allow(ctx, "k", 3, time.Minute); !r.Allowed || r.Remaining != 2 {
		t.Fatalf("a new window must start fresh: %+v", r)
	}
}

func TestMemoryIsSafeForConcurrentUse(t *testing.T) {
	m := NewMemory()
	var wg sync.WaitGroup
	var mu sync.Mutex
	allowed := 0
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if r, _ := m.Allow(context.Background(), "k", 50, time.Minute); r.Allowed {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if allowed != 50 {
		t.Fatalf("exactly 50 requests may pass, got %d", allowed)
	}
}

func TestRedisWindow(t *testing.T) {
	url := os.Getenv("TEST_REDIS_URL")
	if url == "" {
		t.Skip("TEST_REDIS_URL not set")
	}
	opts, err := redis.ParseURL(url)
	if err != nil {
		t.Fatal(err)
	}
	client := redis.NewClient(opts)
	defer client.Close()

	prefix := fmt.Sprintf("test:%d:", time.Now().UnixNano())
	l := NewRedis(client, prefix)
	ctx := context.Background()
	defer client.Del(ctx, prefix+"k")

	for i := 1; i <= 2; i++ {
		if r, err := l.Allow(ctx, "k", 2, 2*time.Second); err != nil || !r.Allowed {
			t.Fatalf("request %d: %+v %v", i, r, err)
		}
	}
	r, err := l.Allow(ctx, "k", 2, 2*time.Second)
	if err != nil || r.Allowed || r.RetryAfter <= 0 || r.RetryAfter > 2*time.Second {
		t.Fatalf("over the limit: %+v %v", r, err)
	}

	time.Sleep(2100 * time.Millisecond)
	if r, _ := l.Allow(ctx, "k", 2, 2*time.Second); !r.Allowed {
		t.Fatal("the window must expire")
	}
}
