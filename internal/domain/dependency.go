package domain

type DependencyType string

const (
	DependencyRolloutOrder        DependencyType = "rollout-order"
	DependencySchemaCompatibility DependencyType = "schema-compatibility"
	DependencyRuntime             DependencyType = "runtime"
	DependencySharedFailureDomain DependencyType = "shared-failure-domain"
	DependencyDataMigration       DependencyType = "data-migration"
	DependencyTraffic             DependencyType = "traffic"
)

func (t DependencyType) Valid() bool {
	switch t {
	case DependencyRolloutOrder,
		DependencySchemaCompatibility,
		DependencyRuntime,
		DependencySharedFailureDomain,
		DependencyDataMigration,
		DependencyTraffic:
		return true
	default:
		return false
	}
}

func (t DependencyType) EnforcesRolloutOrder() bool {
	switch t {
	case DependencyRolloutOrder, DependencyDataMigration, DependencyTraffic:
		return true
	default:
		return false
	}
}

type Dependency struct {
	Service    string         `json:"service,omitempty" yaml:"service,omitempty"`
	Resource   string         `json:"resource,omitempty" yaml:"resource,omitempty"`
	Type       DependencyType `json:"type" yaml:"type"`
	Condition  string         `json:"condition,omitempty" yaml:"condition,omitempty"`
	Constraint string         `json:"constraint,omitempty" yaml:"constraint,omitempty"`
}

type Capability string

const (
	CapabilityDeploy        Capability = "deploy"
	CapabilityGetStatus     Capability = "get_status"
	CapabilityCancel        Capability = "cancel"
	CapabilityRollback      Capability = "rollback"
	CapabilityVerifyRuntime Capability = "verify_runtime"
	CapabilityVerifyMetric  Capability = "verify_metric"
	CapabilityDrain         Capability = "drain"
	CapabilityShiftTraffic  Capability = "shift_traffic"
)

func (c Capability) Valid() bool {
	switch c {
	case CapabilityDeploy,
		CapabilityGetStatus,
		CapabilityCancel,
		CapabilityRollback,
		CapabilityVerifyRuntime,
		CapabilityVerifyMetric,
		CapabilityDrain,
		CapabilityShiftTraffic:
		return true
	default:
		return false
	}
}
