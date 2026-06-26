package naics

import "testing"

// TestDatasetLoaded sanity-checks the embedded 2022 dataset: a realistic code count and a
// known code present with its canonical title.
func TestDatasetLoaded(t *testing.T) {
	all := All()
	if len(all) < 1000 || len(all) > 1100 {
		t.Errorf("loaded %d codes, want ~1012 (the 2022 six-digit edition)", len(all))
	}
	c, ok := Lookup("541512")
	if !ok {
		t.Fatal("541512 (Computer Systems Design Services) not found in the 2022 dataset")
	}
	if c.Title != "Computer Systems Design Services" {
		t.Errorf("541512 title = %q, want canonical Census title", c.Title)
	}
	if _, ok := Lookup("999999"); ok {
		t.Error("999999 should not exist — Lookup must reject non-codes")
	}
}

// TestSearchRanking verifies the typeahead ranking: exact code, code prefix, title-word
// prefix, then substring — and that blank queries return nothing.
func TestSearchRanking(t *testing.T) {
	if got := Search("", 10); got != nil {
		t.Errorf("blank query returned %d results, want nil", len(got))
	}

	// Exact code is the top hit.
	if got := Search("541512", 5); len(got) == 0 || got[0].Code != "541512" {
		t.Errorf("exact-code search top = %v, want 541512 first", got)
	}

	// Code prefix returns the family, all starting with the prefix.
	pre := Search("5415", 50)
	if len(pre) == 0 {
		t.Fatal("code-prefix search 5415 returned nothing")
	}
	for _, c := range pre {
		if len(c.Code) < 4 || c.Code[:4] != "5415" {
			t.Errorf("code-prefix result %s does not start with 5415", c.Code)
		}
	}

	// Title search finds the relevant industry by keyword.
	cyber := Search("computer systems design", 10)
	found := false
	for _, c := range cyber {
		if c.Code == "541512" {
			found = true
		}
	}
	if !found {
		t.Error("title search 'computer systems design' did not surface 541512")
	}

	// limit is honored.
	if got := Search("services", 3); len(got) > 3 {
		t.Errorf("limit=3 returned %d results", len(got))
	}
}
