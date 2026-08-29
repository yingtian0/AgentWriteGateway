package protocol

type Capability string

const (
	CapabilityDeploy   Capability = "release.deploy"
	CapabilityRollback Capability = "release.rollback"
)

type Action struct {
	Capability          Capability `json:"capability"`
	ArtifactDigest      string     `json:"artifact_digest"`
	ExternalExecutionID string     `json:"external_execution_id,omitempty"`
}

type Target struct {
	Service     string `json:"service"`
	Environment string `json:"environment"`
}

type Capabilities struct {
	ProtocolVersions []string     `json:"protocol_versions"`
	Actions          []Capability `json:"actions"`
	RunnerGroup      string       `json:"runner_group"`
}
