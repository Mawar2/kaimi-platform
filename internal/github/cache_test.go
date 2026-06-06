package github

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestNewCache_DefaultTTLs verifies that New() sets the expected default TTLs.
func TestNewCache_DefaultTTLs(t *testing.T) {
	c := New()

	if c.issueTTL != 5*time.Minute {
		t.Errorf("expected issueTTL %v, got %v", 5*time.Minute, c.issueTTL)
	}
	if c.prTTL != 2*time.Minute {
		t.Errorf("expected prTTL %v, got %v", 2*time.Minute, c.prTTL)
	}
}

// TestNewCacheWithTTL verifies that NewWithTTL sets custom TTLs correctly.
func TestNewCacheWithTTL(t *testing.T) {
	c := NewWithTTL(10*time.Minute, 4*time.Minute)

	if c.issueTTL != 10*time.Minute {
		t.Errorf("expected issueTTL %v, got %v", 10*time.Minute, c.issueTTL)
	}
	if c.prTTL != 4*time.Minute {
		t.Errorf("expected prTTL %v, got %v", 4*time.Minute, c.prTTL)
	}
}

// TestCache_SetGetIssue verifies basic set and get for issues.
func TestCache_SetGetIssue(t *testing.T) {
	c := New()
	issues := []*Issue{
		{Number: 1, Title: "Fix authentication bug", State: "open"},
		{Number: 2, Title: "Add caching layer", State: "open"},
	}

	c.SetIssue("Mawar2/Kaimi:open", issues)

	got, ok := c.GetIssue("Mawar2/Kaimi:open")
	if !ok {
		t.Fatal("expected to find issues in cache, got miss")
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(got))
	}
	if got[0].Number != 1 {
		t.Errorf("expected issue #1, got #%d", got[0].Number)
	}
	if got[1].Title != "Add caching layer" {
		t.Errorf("expected title %q, got %q", "Add caching layer", got[1].Title)
	}
}

// TestCache_GetIssue_Miss verifies a cache miss returns nil, false.
func TestCache_GetIssue_Miss(t *testing.T) {
	c := New()

	got, ok := c.GetIssue("nonexistent-key")
	if ok {
		t.Error("expected cache miss, got hit")
	}
	if got != nil {
		t.Errorf("expected nil on miss, got %v", got)
	}
}

// TestCache_SetGetPR verifies basic set and get for pull requests.
func TestCache_SetGetPR(t *testing.T) {
	c := New()
	prs := []*PullRequest{
		{Number: 10, Title: "Implement caching", State: "open", Draft: false, HeadRef: "feature/cache", BaseRef: "main"},
	}

	c.SetPR("Mawar2/Kaimi:open", prs)

	got, ok := c.GetPR("Mawar2/Kaimi:open")
	if !ok {
		t.Fatal("expected to find PRs in cache, got miss")
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 PR, got %d", len(got))
	}
	if got[0].Number != 10 {
		t.Errorf("expected PR #10, got #%d", got[0].Number)
	}
	if got[0].HeadRef != "feature/cache" {
		t.Errorf("expected head ref %q, got %q", "feature/cache", got[0].HeadRef)
	}
}

// TestCache_GetPR_Miss verifies a PR cache miss returns nil, false.
func TestCache_GetPR_Miss(t *testing.T) {
	c := New()

	got, ok := c.GetPR("nonexistent-key")
	if ok {
		t.Error("expected cache miss, got hit")
	}
	if got != nil {
		t.Errorf("expected nil on miss, got %v", got)
	}
}

// TestCache_Issue_TTLExpiry verifies that issue entries expire after their TTL.
func TestCache_Issue_TTLExpiry(t *testing.T) {
	// Use a very short TTL to make expiry testable without long sleeps.
	c := NewWithTTL(50*time.Millisecond, 5*time.Minute)

	issues := []*Issue{{Number: 42, Title: "Expiring issue", State: "open"}}
	c.SetIssue("expiry-key", issues)

	// Entry should be present immediately.
	_, ok := c.GetIssue("expiry-key")
	if !ok {
		t.Fatal("expected cache hit immediately after set")
	}

	// Wait for TTL to elapse.
	time.Sleep(100 * time.Millisecond)

	// Entry should now be expired.
	got, ok := c.GetIssue("expiry-key")
	if ok {
		t.Error("expected cache miss after TTL expiry, got hit")
	}
	if got != nil {
		t.Errorf("expected nil after expiry, got %v", got)
	}
}

// TestCache_PR_TTLExpiry verifies that PR entries expire after their TTL.
func TestCache_PR_TTLExpiry(t *testing.T) {
	c := NewWithTTL(5*time.Minute, 50*time.Millisecond)

	prs := []*PullRequest{{Number: 7, Title: "Expiring PR", State: "open"}}
	c.SetPR("expiry-pr-key", prs)

	_, ok := c.GetPR("expiry-pr-key")
	if !ok {
		t.Fatal("expected cache hit immediately after set")
	}

	time.Sleep(100 * time.Millisecond)

	got, ok := c.GetPR("expiry-pr-key")
	if ok {
		t.Error("expected cache miss after TTL expiry, got hit")
	}
	if got != nil {
		t.Errorf("expected nil after expiry, got %v", got)
	}
}

// TestCache_InvalidateIssue verifies that InvalidateIssue removes a specific entry.
func TestCache_InvalidateIssue(t *testing.T) {
	c := New()
	c.SetIssue("key-a", []*Issue{{Number: 1, Title: "Issue A"}})
	c.SetIssue("key-b", []*Issue{{Number: 2, Title: "Issue B"}})

	c.InvalidateIssue("key-a")

	if _, ok := c.GetIssue("key-a"); ok {
		t.Error("expected key-a to be invalidated")
	}
	if _, ok := c.GetIssue("key-b"); !ok {
		t.Error("expected key-b to still be present after invalidating key-a")
	}
}

// TestCache_InvalidatePR verifies that InvalidatePR removes a specific entry.
func TestCache_InvalidatePR(t *testing.T) {
	c := New()
	c.SetPR("pr-key-a", []*PullRequest{{Number: 10, Title: "PR A"}})
	c.SetPR("pr-key-b", []*PullRequest{{Number: 11, Title: "PR B"}})

	c.InvalidatePR("pr-key-a")

	if _, ok := c.GetPR("pr-key-a"); ok {
		t.Error("expected pr-key-a to be invalidated")
	}
	if _, ok := c.GetPR("pr-key-b"); !ok {
		t.Error("expected pr-key-b to still be present after invalidating pr-key-a")
	}
}

// TestCache_Invalidate verifies that Invalidate clears all entries.
func TestCache_Invalidate(t *testing.T) {
	c := New()
	c.SetIssue("issues-key", []*Issue{{Number: 1, Title: "Some issue"}})
	c.SetPR("prs-key", []*PullRequest{{Number: 5, Title: "Some PR"}})

	c.Invalidate()

	if _, ok := c.GetIssue("issues-key"); ok {
		t.Error("expected issues to be cleared after Invalidate()")
	}
	if _, ok := c.GetPR("prs-key"); ok {
		t.Error("expected PRs to be cleared after Invalidate()")
	}
}

// TestCache_Overwrite verifies that setting the same key overwrites the previous value.
func TestCache_Overwrite(t *testing.T) {
	c := New()

	c.SetIssue("key", []*Issue{{Number: 1, Title: "Original"}})
	c.SetIssue("key", []*Issue{{Number: 2, Title: "Updated"}})

	got, ok := c.GetIssue("key")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got[0].Number != 2 {
		t.Errorf("expected overwritten value #2, got #%d", got[0].Number)
	}
}

// TestCache_EmptySlice verifies that caching an empty slice is valid and retrievable.
func TestCache_EmptySlice(t *testing.T) {
	c := New()
	c.SetIssue("empty-key", []*Issue{})

	got, ok := c.GetIssue("empty-key")
	if !ok {
		t.Fatal("expected cache hit for empty slice")
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %d elements", len(got))
	}
}

// TestCache_IssuesAndPRsSeparate verifies that issue and PR caches don't interfere.
func TestCache_IssuesAndPRsSeparate(t *testing.T) {
	c := New()

	c.SetIssue("shared-key", []*Issue{{Number: 1, Title: "Issue"}})
	c.SetPR("shared-key", []*PullRequest{{Number: 10, Title: "PR"}})

	issues, ok := c.GetIssue("shared-key")
	if !ok || issues[0].Number != 1 {
		t.Error("issue cache corrupted after setting PR with same key")
	}

	prs, ok := c.GetPR("shared-key")
	if !ok || prs[0].Number != 10 {
		t.Error("PR cache corrupted after setting issue with same key")
	}
}

// TestCache_Cleanup verifies that Cleanup removes expired entries without touching live ones.
func TestCache_Cleanup(t *testing.T) {
	c := NewWithTTL(50*time.Millisecond, 50*time.Millisecond)

	// Set entries that will expire.
	c.SetIssue("expire-issues", []*Issue{{Number: 1, Title: "Expiring"}})
	c.SetPR("expire-prs", []*PullRequest{{Number: 2, Title: "Expiring"}})

	// Wait for expiry.
	time.Sleep(100 * time.Millisecond)

	// Set a fresh entry that should survive cleanup.
	c.SetIssue("fresh-issues", []*Issue{{Number: 3, Title: "Fresh"}})

	c.Cleanup()

	// Expired entries should be gone from internal maps (not just returning miss on Get).
	c.mu.RLock()
	_, expiredIssuePresent := c.issues["expire-issues"]
	_, freshIssuePresent := c.issues["fresh-issues"]
	c.mu.RUnlock()

	if expiredIssuePresent {
		t.Error("expected expired issue entry to be removed by Cleanup()")
	}
	if !freshIssuePresent {
		t.Error("expected fresh issue entry to survive Cleanup()")
	}
}

// TestCache_ConcurrentAccess verifies thread-safety under concurrent reads and writes.
func TestCache_ConcurrentAccess(t *testing.T) {
	c := New()
	var wg sync.WaitGroup

	// Concurrent writers.
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			key := fmt.Sprintf("key-%d", id)
			c.SetIssue(key, []*Issue{{Number: id, Title: fmt.Sprintf("Issue %d", id)}})
			c.SetPR(key, []*PullRequest{{Number: id, Title: fmt.Sprintf("PR %d", id)}})
		}(i)
	}

	// Concurrent readers (reading keys that may or may not exist yet).
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			key := fmt.Sprintf("key-%d", id)
			c.GetIssue(key) // return values intentionally ignored in concurrent test
			c.GetPR(key)
		}(i)
	}

	// Concurrent invalidations.
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			c.InvalidateIssue(fmt.Sprintf("key-%d", id))
			c.InvalidatePR(fmt.Sprintf("key-%d", id))
		}(i)
	}

	wg.Wait()
	// No race detector errors means success.
}

// TestCache_ConcurrentCleanup verifies Cleanup() is safe when called concurrently with writes.
func TestCache_ConcurrentCleanup(t *testing.T) {
	c := NewWithTTL(10*time.Millisecond, 10*time.Millisecond)
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			c.SetIssue(fmt.Sprintf("k%d", id), []*Issue{{Number: id}})
			time.Sleep(15 * time.Millisecond)
			c.Cleanup()
		}(i)
	}

	wg.Wait()
}
