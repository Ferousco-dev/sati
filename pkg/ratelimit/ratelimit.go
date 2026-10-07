package ratelimit

import (
	"context"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type Result struct {
	Allowed    bool
	Remaining  int
	RetryAfter time.Duration
}

type Limiter interface {
	// Allow counts one request for key and says whether it is within limit per window.
	Allow(ctx context.Context, key string, limit int, window time.Duration) (Result, error)
}

type bucket struct {
	count   int
	resetAt time.Time
}

// Memory keeps counters in this process. It is used when Redis is not configured,
// so each API instance counts on its own and counts reset on restart.
type Memory struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	now     func() time.Time
}

func NewMemory() *Memory { return &Memory{buckets: map[string]*bucket{}, now: time.Now} }

func (m *Memory) Allow(_ context.Context, key string, limit int, window time.Duration) (Result, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.now()
	if len(m.buckets) > 10_000 {
		for k, b := range m.buckets {
			if !b.resetAt.After(now) {
				delete(m.buckets, k)
			}
		}
	}

	b := m.buckets[key]
	if b == nil || !b.resetAt.After(now) {
		b = &bucket{resetAt: now.Add(window)}
		m.buckets[key] = b
	}
	b.count++
	return result(b.count, limit, b.resetAt.Sub(now)), nil
}

func result(count, limit int, ttl time.Duration) Result {
	remaining := limit - count
	if remaining < 0 {
		remaining = 0
	}
	return Result{Allowed: count <= limit, Remaining: remaining, RetryAfter: ttl}
}

var windowScript = redis.NewScript(`
local count = redis.call('INCR', KEYS[1])
local ttl = redis.call('PTTL', KEYS[1])
if count == 1 or ttl < 0 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
  ttl = tonumber(ARGV[1])
end
return {count, ttl}
`)

// Redis shares counters between every API instance.
type Redis struct {
	client redis.Scripter
	prefix string
}

func NewRedis(client redis.Scripter, prefix string) *Redis {
	return &Redis{client: client, prefix: prefix}
}

func (r *Redis) Allow(ctx context.Context, key string, limit int, window time.Duration) (Result, error) {
	out, err := windowScript.Run(ctx, r.client, []string{r.prefix + key}, window.Milliseconds()).Int64Slice()
	if err != nil {
		return Result{}, err
	}
	return result(int(out[0]), limit, time.Duration(out[1])*time.Millisecond), nil
}
