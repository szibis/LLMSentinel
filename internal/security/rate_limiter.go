package security

import (
	"sync"
	"time"
)

// RateLimiter implements token bucket rate limiting
type RateLimiter struct {
	requestsPerMinute int
	perIP             bool
	ipBuckets         map[string]*tokenBucket
	mu                sync.RWMutex
	cleanupTicker     *time.Ticker
}

// tokenBucket represents a token bucket for rate limiting
type tokenBucket struct {
	tokens    float64
	lastRefill time.Time
	mu        sync.Mutex
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(requestsPerMinute int, perIP bool) *RateLimiter {
	rl := &RateLimiter{
		requestsPerMinute: requestsPerMinute,
		perIP:             perIP,
		ipBuckets:         make(map[string]*tokenBucket),
		cleanupTicker:     time.NewTicker(5 * time.Minute),
	}

	// Start cleanup goroutine
	go rl.cleanupExpiredBuckets()

	return rl
}

// Allow checks if a request is allowed
func (rl *RateLimiter) Allow(ip string) bool {
	if !rl.perIP {
		ip = "global"
	}

	rl.mu.Lock()
	bucket, exists := rl.ipBuckets[ip]
	if !exists {
		bucket = &tokenBucket{
			tokens:    float64(rl.requestsPerMinute),
			lastRefill: time.Now(),
		}
		rl.ipBuckets[ip] = bucket
	}
	rl.mu.Unlock()

	bucket.mu.Lock()
	defer bucket.mu.Unlock()

	// Calculate tokens to add since last refill
	now := time.Now()
	elapsed := now.Sub(bucket.lastRefill).Seconds()
	tokensToAdd := elapsed * float64(rl.requestsPerMinute) / 60.0

	if tokensToAdd > 0 {
		bucket.tokens = min(bucket.tokens+tokensToAdd, float64(rl.requestsPerMinute))
		bucket.lastRefill = now
	}

	if bucket.tokens >= 1.0 {
		bucket.tokens -= 1.0
		return true
	}

	return false
}

// GetRemaining returns the number of remaining requests for an IP
func (rl *RateLimiter) GetRemaining(ip string) int {
	if !rl.perIP {
		ip = "global"
	}

	rl.mu.RLock()
	bucket, exists := rl.ipBuckets[ip]
	rl.mu.RUnlock()

	if !exists {
		return rl.requestsPerMinute
	}

	bucket.mu.Lock()
	defer bucket.mu.Unlock()

	return int(bucket.tokens)
}

// Reset resets the limiter for an IP
func (rl *RateLimiter) Reset(ip string) {
	if !rl.perIP {
		ip = "global"
	}

	rl.mu.Lock()
	delete(rl.ipBuckets, ip)
	rl.mu.Unlock()
}

// cleanupExpiredBuckets periodically cleans up old buckets
func (rl *RateLimiter) cleanupExpiredBuckets() {
	for range rl.cleanupTicker.C {
		rl.mu.Lock()

		now := time.Now()
		for ip, bucket := range rl.ipBuckets {
			bucket.mu.Lock()
			if now.Sub(bucket.lastRefill) > 10*time.Minute {
				delete(rl.ipBuckets, ip)
			}
			bucket.mu.Unlock()
		}

		rl.mu.Unlock()
	}
}

// Close stops the rate limiter
func (rl *RateLimiter) Close() {
	rl.cleanupTicker.Stop()
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
