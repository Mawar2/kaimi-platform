package httpapi

import (
	"net/http"
	"strconv"

	"github.com/Mawar2/Kaimi/internal/naics"
)

// naicsResponse wraps the typeahead results. Always a (possibly empty) array, never null.
type naicsResponse struct {
	Results []naics.Code `json:"results"`
}

// handleSearchNAICS serves GET /api/v1/naics?q=<query>&limit=<n>. It returns ranked matches
// from the official 2022 NAICS taxonomy so onboarding's typeahead yields a REAL code with its
// canonical title — which becomes the hunt's `ncode` filter — instead of a free-typed code
// that could be wrong or misformatted. Read-only; runs inside the authenticated /api/v1 group.
func (s *Server) handleSearchNAICS(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")

	limit := 0 // naics.Search applies its default
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 50 {
			limit = n
		}
	}

	results := naics.Search(q, limit)
	if results == nil {
		results = []naics.Code{} // emit [] not null for a blank/no-match query
	}
	writeJSON(w, http.StatusOK, naicsResponse{Results: results})
}
