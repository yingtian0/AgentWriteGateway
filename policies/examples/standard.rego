package themisy.authorization

import rego.v1

# Environment/team/service modules only add deny reasons. They cannot remove a
# platform mandatory deny, which makes policy composition monotonic.
deny contains "PRODUCTION_RISK_NOT_APPROVED" if {
	input.environment == "production"
	input.risk == "medium"
	count(object.get(input, "approval_proofs", [])) == 0
}
