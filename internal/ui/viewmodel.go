package ui

import "themisy/internal/domain"

type runView struct {
	Title  string
	Run    *domain.ReleaseRun
	Events []domain.AuditEvent
}

type approvalView struct {
	Title     string
	Approvals []domain.ApprovalSummary
	CSRFToken string
}
