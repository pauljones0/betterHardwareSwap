package discord

import (
	"regexp"
	"strings"
	"sync"
	"time"
)

// RateLimiter provides a simple in-memory token bucket rate limiter.
type RateLimiter struct {
	mu       sync.Mutex
	lastSeen map[string]time.Time
}

func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		lastSeen: make(map[string]time.Time),
	}
}

// Allow checks if the given userID is allowed to perform an action (max 1 request per 2 seconds).
func (rl *RateLimiter) Allow(userID string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	last, ok := rl.lastSeen[userID]
	if ok && time.Since(last) < 2*time.Second {
		return false
	}

	rl.lastSeen[userID] = time.Now()
	return true
}

var (
	// regex to strip potentially dangerous characters while allowing common hardware/location characters.
	sanitizeRegex = regexp.MustCompile(`[^a-zA-Z0-9\s.,!?-]`)
)

// Sanitize cleans up user input strings to prevent basic injection or formatting abuse.
func Sanitize(input string) string {
	// 1. Limit length
	if len(input) > 500 {
		input = input[:500]
	}

	// 2. Strip dangerous characters
	input = sanitizeRegex.ReplaceAllString(input, "")

	// 3. Trim whitespace
	return strings.TrimSpace(input)
}
