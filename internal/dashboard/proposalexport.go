package dashboard

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/Mawar2/Kaimi/internal/document"
	"github.com/Mawar2/Kaimi/internal/export"
)

// ErrDriveNotConnected is returned by a ProposalDriveSaver when the tenant has not connected
// their Google Drive yet, so the handler can redirect them to the Connect step instead of
// erroring. cmd/api maps the drivetoken "not connected" error to this.
var ErrDriveNotConnected = errors.New("dashboard: customer Google Drive is not connected")

// ProposalDriveSaver writes the finished proposal into the tenant's connected Google Drive as
// an editable Google Doc and returns the Doc's URL. cmd/api implements it over the existing
// customer-Drive token + target folder + the googledocs client (no new API).
type ProposalDriveSaver func(ctx context.Context, doc *document.Document) (docURL string, err error)

// WithProposalDriveSaver enables the workspace "Save to Google Drive" action. Without it, the
// action is hidden.
func WithProposalDriveSaver(fn ProposalDriveSaver) Option {
	return func(h *Handler) { h.saveToDrive = fn }
}

// handleSaveToDrive serves POST /workspace/{id}/save-to-drive: it creates an editable Google
// Doc of the proposal in the tenant's connected Drive and redirects them straight into that
// Doc (to tweak + share). FAILS CLOSED on auth + CSRF. When Drive isn't connected it sends
// them to the onboarding Connect step instead of erroring.
func (h *Handler) handleSaveToDrive(w http.ResponseWriter, r *http.Request) {
	if h.saveToDrive == nil {
		http.Error(w, "Google Drive save is not available on this server", http.StatusServiceUnavailable)
		return
	}
	// Session-gated like the other workspace POSTs (approve/changes/submit); the __Host-
	// session cookie's SameSite=Lax is the cross-site-POST defense, so no separate CSRF token.
	id := r.PathValue("id")
	if !opportunityIDPattern.MatchString(id) {
		h.renderNotFound(w, id)
		return
	}
	doc, err := h.proposals.Document(id)
	if err != nil {
		h.renderNotFound(w, id)
		return
	}
	url, err := h.saveToDrive(r.Context(), doc)
	if err != nil {
		if errors.Is(err, ErrDriveNotConnected) {
			http.Redirect(w, r, onboardingPath+"?step=connect", http.StatusSeeOther)
			return
		}
		log.Printf("dashboard: save proposal to Drive failed: %v", err)
		http.Error(w, "failed to save to Google Drive", http.StatusInternalServerError)
		return
	}
	// Land the tenant directly in their new editable Doc in Drive.
	http.Redirect(w, r, url, http.StatusSeeOther)
}

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

// handleProposalComplianceCSV serves the proposal's compliance matrix as a CSV download — one
// row per must-have requirement (extracted by the Scorer) traced to its coverage: Addressed
// in a section, a GAP named by the Final Review, or left for the user to Review. When no
// requirements were extracted it still returns a usable manual template, never an error page.
func (h *Handler) handleProposalComplianceCSV(w http.ResponseWriter, r *http.Request) {
	doc, id, ok := h.loadExportDoc(w, r)
	if !ok {
		return
	}
	var reqs []string
	if opp, err := h.svc.Get(r.Context(), id); err == nil && opp != nil {
		reqs = opp.Requirements
	}
	data, err := export.RenderComplianceCSV(doc, reqs, h.exportOptions(r.Context(), id))
	if err != nil {
		http.Error(w, "failed to render the compliance matrix", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", proposalFilename(doc, id, "csv")))
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
