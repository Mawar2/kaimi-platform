// Package github provides an in-memory caching layer for GitHub REST API responses.
//
// The Cache type stores issues and pull requests with configurable per-type TTLs.
// Default TTLs follow the project's latency/cost trade-off: 5 minutes for issues
// (change infrequently) and 2 minutes for pull requests (CI status changes more often).
//
// All Cache methods are safe for concurrent use.
package github

import (
	"sync"
	"time"
)

// IssueTTL is the default time-to-live for cached GitHub issue lists.
const IssueTTL = 5 * time.Minute

// PRTTL is the default time-to-live for cached GitHub pull request lists.
const PRTTL = 2 * time.Minute

// Issue represents a GitHub issue returned by the REST API.
type Issue struct {
	Number    int
	Title     string
	Body      string
	State     string // "open" or "closed"
	Labels    []string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// PullRequest represents a GitHub pull request returned by the REST API.
type PullRequest struct {
	Number    int
	Title     string
	Body      string
	State     string // "open", "closed"
	Draft     bool
	HeadRef   string
	BaseRef   string
	MergedAt  time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

// issueEntry holds a cached issue list with its expiry timestamp.
type issueEntry struct {
	value     []*Issue
	expiresAt time.Time
}

// prEntry holds a cached pull request list with its expiry timestamp.
type prEntry struct {
	value     []*PullRequest
	expiresAt time.Time
}

// Cache is a thread-safe in-memory cache for GitHub API responses.
//
// Issues and pull requests are stored in separate maps keyed by an arbitrary
// string (typically "<owner>/<repo>:<filter>"). Each map has its own TTL
// configured at construction time.
type Cache struct {
	mu       sync.RWMutex
	issues   map[string]issueEntry
	prs      map[string]prEntry
	issueTTL time.Duration
	prTTL    time.Duration
}

// New returns a Cache with the default TTLs: 5 minutes for issues, 2 minutes for PRs.
func New() *Cache {
	return NewWithTTL(IssueTTL, PRTTL)
}

// NewWithTTL returns a Cache with the given per-type TTLs.
// Use this in tests or when the defaults don't fit your polling interval.
func NewWithTTL(issueTTL, prTTL time.Duration) *Cache {
	return &Cache{
		issues:   make(map[string]issueEntry),
		prs:      make(map[string]prEntry),
		issueTTL: issueTTL,
		prTTL:    prTTL,
	}
}

// GetIssue returns the cached issue list for key and true if a non-expired entry exists.
// Returns nil, false on a cache miss or after TTL expiry.
func (c *Cache) GetIssue(key string) ([]*Issue, bool) {
	c.mu.RLock()
	entry, ok := c.issues[key]
	c.mu.RUnlock()

	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.value, true
}

// SetIssue stores issues under key with the issue TTL.
// Overwrites any existing entry for the same key.
func (c *Cache) SetIssue(key string, issues []*Issue) {
	c.mu.Lock()
	c.issues[key] = issueEntry{
		value:     issues,
		expiresAt: time.Now().Add(c.issueTTL),
	}
	c.mu.Unlock()
}

// GetPR returns the cached pull request list for key and true if a non-expired entry exists.
// Returns nil, false on a cache miss or after TTL expiry.
func (c *Cache) GetPR(key string) ([]*PullRequest, bool) {
	c.mu.RLock()
	entry, ok := c.prs[key]
	c.mu.RUnlock()

	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.value, true
}

// SetPR stores pull requests under key with the PR TTL.
// Overwrites any existing entry for the same key.
func (c *Cache) SetPR(key string, prs []*PullRequest) {
	c.mu.Lock()
	c.prs[key] = prEntry{
		value:     prs,
		expiresAt: time.Now().Add(c.prTTL),
	}
	c.mu.Unlock()
}

// InvalidateIssue removes the cached issues for key, if present.
func (c *Cache) InvalidateIssue(key string) {
	c.mu.Lock()
	delete(c.issues, key)
	c.mu.Unlock()
}

// InvalidatePR removes the cached pull requests for key, if present.
func (c *Cache) InvalidatePR(key string) {
	c.mu.Lock()
	delete(c.prs, key)
	c.mu.Unlock()
}

// Invalidate clears all cached issues and pull requests.
func (c *Cache) Invalidate() {
	c.mu.Lock()
	c.issues = make(map[string]issueEntry)
	c.prs = make(map[string]prEntry)
	c.mu.Unlock()
}

// Cleanup removes all entries whose TTL has elapsed.
// Call this periodically if the cache is long-lived and holds many keys, to
// prevent unbounded memory growth from expired-but-unretrieved entries.
func (c *Cache) Cleanup() {
	now := time.Now()

	c.mu.Lock()
	defer c.mu.Unlock()

	for key, entry := range c.issues {
		if now.After(entry.expiresAt) {
			delete(c.issues, key)
		}
	}
	for key, entry := range c.prs {
		if now.After(entry.expiresAt) {
			delete(c.prs, key)
		}
	}
}
