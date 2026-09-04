package audit

import "themisy/pkg/adapter"

// EvidenceDetails intentionally omits raw queries, credentials, and log text.
func EvidenceDetails(evidence adapter.Evidence) map[string]any {
	return map[string]any{
		"status":          evidence.Status,
		"reason_code":     evidence.ReasonCode,
		"source":          evidence.Source,
		"query_hash":      evidence.QueryHash,
		"window_from":     evidence.Window.From,
		"window_to":       evidence.Window.To,
		"observed_at":     evidence.ObservedAt,
		"observed_value":  evidence.ObservedValue,
		"threshold":       evidence.Threshold,
		"adapter_version": evidence.AdapterVersion,
		"evidence_hash":   evidence.EvidenceHash,
	}
}
