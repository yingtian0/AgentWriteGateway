package agentwritegateway.authorization

import rego.v1

test_baseline_allows_complete_staging_input if {
	result := decision with input as {
		"version": "awg.policy.input/v1alpha1",
		"user_id": "user-1",
		"agent_id": "agent-1",
		"delegation_ref": "delegation-1",
		"environment": "staging",
		"risk": "low",
	}
	result.allow
}

test_baseline_denies_missing_subject if {
	result := decision with input as {
		"version": "awg.policy.input/v1alpha1",
		"user_id": "",
		"agent_id": "agent-1",
		"delegation_ref": "delegation-1",
		"environment": "staging",
		"risk": "low",
	}
	not result.allow
	"MISSING_SUBJECT" in result.reasons
}
