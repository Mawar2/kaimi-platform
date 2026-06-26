package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHandleSearchNAICS exercises the typeahead endpoint: a query returns ranked JSON
// results including the expected code, and a blank query returns an empty array (not null).
func TestHandleSearchNAICS(t *testing.T) {
	srv := &Server{} // handler reads only the query + the embedded dataset, not s.deps

	t.Run("query returns results", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/naics?q=computer+systems+design&limit=5", http.NoBody)
		rec := httptest.NewRecorder()
		srv.handleSearchNAICS(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		var resp naicsResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("bad JSON: %v", err)
		}
		if len(resp.Results) == 0 || len(resp.Results) > 5 {
			t.Fatalf("got %d results, want 1..5", len(resp.Results))
		}
		found := false
		for _, c := range resp.Results {
			if c.Code == "541512" && c.Title != "" {
				found = true
			}
		}
		if !found {
			t.Errorf("results did not include 541512 with a title: %+v", resp.Results)
		}
	})

	t.Run("blank query returns empty array not null", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/naics", http.NoBody)
		rec := httptest.NewRecorder()
		srv.handleSearchNAICS(rec, req)
		if got := rec.Body.String(); !contains(got, `"results":[]`) {
			t.Errorf("blank query body = %q, want results:[]", got)
		}
	})
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
