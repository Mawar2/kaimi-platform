package dashboard

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/Mawar2/Kaimi/internal/document"
	"github.com/Mawar2/Kaimi/internal/export"
)

// handleProposalDOCX serves the proposal as a Microsoft Word (.docx) download — the editable
// copy a tenant shares with teammates/partners and revises before submission. It renders from
// the same structured document as the PDF, preserving section order, headings, and [GAP: …]
// markers. The internal document.json is never exposed.
func (h *Handler) handleProposalDOCX(w http.ResponseWriter, r *http.Request) {
	doc, id, ok := h.loadExportDoc(w, r)
	if !ok {
		return
	}
	data, err := export.RenderDOCX(doc, h.exportOptions(r.Context(), id))
	if err != nil {
		http.Error(w, "failed to render the Word document", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", proposalFilename(doc, id, "docx")))
	_, _ = w.Write(data)
}

// handleProposalPDF serves the proposal as a PDF download — the locked copy for the actual
// submission. Same content/structure as the Word export.
func (h *Handler) handleProposalPDF(w http.ResponseWriter, r *http.Request) {
	doc, id, ok := h.loadExportDoc(w, r)
	if !ok {
		return
	}
	data, err := export.RenderPDF(doc, h.exportOptions(r.Context(), id))
	if err != nil {
		http.Error(w, "failed to render the PDF", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", proposalFilename(doc, id, "pdf")))
	_, _ = w.Write(data)
}

// loadExportDoc validates the request + loads the proposal document, writing the appropriate
// error response and returning ok=false on any failure.
func (h *Handler) loadExportDoc(w http.ResponseWriter, r *http.Request) (*document.Document, string, bool) {
	if h.proposals == nil {
		http.Error(w, "proposal export is not enabled on this server", http.StatusServiceUnavailable)
		return nil, "", false
	}
	id := r.PathValue("id")
	if !opportunityIDPattern.MatchString(id) {
		h.renderNotFound(w, id)
		return nil, "", false
	}
	doc, err := h.proposals.Document(id)
	if err != nil {
		h.renderNotFound(w, id)
		return nil, "", false
	}
	return doc, id, true
}

// exportOptions builds the cover-page options from the saved company profile (company name)
// and the opportunity (solicitation number). Both are best-effort: a missing profile/opp
// leaves the field blank, and the renderers fall back to neutral defaults.
func (h *Handler) exportOptions(ctx context.Context, id string) export.Options {
	opts := export.Options{Date: h.Now()}
	if h.profileStore != nil {
		if p, err := h.profileStore.Load(); err == nil && p != nil {
			opts.CompanyName = p.Company
		}
	}
	if opp, err := h.svc.Get(ctx, id); err == nil && opp != nil {
		opts.SolicitationNumber = opp.SolicitationNum
	}
	return opts
}

// filenameUnsafe matches characters not allowed in a clean download filename.
var filenameUnsafe = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// proposalFilename builds a tidy download name from the proposal title (or the id when the
// title is empty), e.g. "Website-Modernization.docx".
func proposalFilename(doc *document.Document, id, ext string) string {
	base := id
	if doc != nil {
		if t := strings.TrimSpace(doc.Title); t != "" {
			base = t
		}
	}
	base = strings.Trim(filenameUnsafe.ReplaceAllString(base, "-"), "-")
	if base == "" {
		base = "proposal"
	}
	if len(base) > 80 {
		base = strings.Trim(base[:80], "-")
	}
	return base + "." + ext
}
