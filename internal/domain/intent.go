package domain

const (
	ReleaseIntentAPIVersion = "execution.themisy.io/v1alpha1"
	ReleaseIntentKind       = "ReleaseIntent"
)

type ReleaseIntent struct {
	APIVersion     string        `json:"api_version"`
	Kind           string        `json:"kind"`
	RequestID      string        `json:"request_id"`
	ReleaseVersion string        `json:"release_version"`
	Environment    Environment   `json:"environment"`
	RequestedBy    string        `json:"requested_by"`
	Agent          AgentIdentity `json:"delegated_agent"`
	Changes        []Change      `json:"changes"`
}
