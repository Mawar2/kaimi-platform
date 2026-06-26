package kobs

import "github.com/Mawar2/Kaimi/internal/opportunity"

// TenantID resolves the tenant an event should be scoped to, preferring the
// per-record value over the deployment default.
//
// Precedence: the Opportunity's own TenantID wins when it is non-empty (newer
// records carry it); otherwise the deployment's configured tenant (typically
// config.Tenant.ID, passed in as cfgTenant) is used; if neither is set the
// result is the empty string, which the event envelope omits. A nil opp is
// treated as having no record-level tenant.
func TenantID(opp *opportunity.Opportunity, cfgTenant string) string {
	if opp != nil && opp.TenantID != "" {
		return opp.TenantID
	}
	return cfgTenant
}
