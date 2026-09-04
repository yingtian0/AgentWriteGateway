package profile

import "themisy/internal/domain"

const (
	APIVersion = "execution.themisy.io/v1alpha1"
	Kind       = "ReleaseProfile"
)

type ReleaseProfile struct {
	APIVersion  string   `json:"apiVersion" yaml:"apiVersion"`
	Kind        string   `json:"kind" yaml:"kind"`
	Metadata    Metadata `json:"metadata" yaml:"metadata"`
	Spec        Spec     `json:"spec" yaml:"spec"`
	ContentHash string   `json:"-" yaml:"-"`
	Source      string   `json:"-" yaml:"-"`
}

type Metadata struct {
	Name string `json:"name" yaml:"name"`
}

type Spec struct {
	RequiredCapabilities []domain.Capability `json:"requiredCapabilities" yaml:"requiredCapabilities"`
	Deployment           Deployment          `json:"deployment" yaml:"deployment"`
	Verification         Verification        `json:"verification" yaml:"verification"`
	Rollback             Rollback            `json:"rollback" yaml:"rollback"`
}

type Deployment struct {
	Strategy string `json:"strategy" yaml:"strategy"`
}

type Verification struct {
	Required          bool   `json:"required" yaml:"required"`
	ObservationWindow string `json:"observationWindow" yaml:"observationWindow"`
}

type Rollback struct {
	Mode string `json:"mode" yaml:"mode"`
}
