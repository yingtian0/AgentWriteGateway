package githubactions

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const (
	AdapterName    = "github-actions"
	AdapterVersion = "github-actions/v1"
	apiBaseURL     = "https://api.github.com"
	apiVersion     = "2026-03-10"
)

var (
	repositoryPart = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
	workflowFile   = regexp.MustCompile(`^[A-Za-z0-9_.-]+\.ya?ml$`)
	gitRef         = regexp.MustCompile(`^[A-Za-z0-9_./-]+$`)
)

type TargetKey struct {
	Service     string
	Environment string
}

// Target is trusted Runner configuration. Workflow files and refs are never
// accepted from an Action Grant or agent-provided request.
type Target struct {
	Owner            string `yaml:"owner" json:"owner"`
	Repository       string `yaml:"repository" json:"repository"`
	DeployWorkflow   string `yaml:"deploy_workflow" json:"deploy_workflow"`
	RollbackWorkflow string `yaml:"rollback_workflow" json:"rollback_workflow"`
	Ref              string `yaml:"ref" json:"ref"`
}

type Config struct {
	Targets map[TargetKey]Target
}

func (c Config) Validate() error {
	if len(c.Targets) == 0 {
		return errors.New("at least one GitHub Actions target is required")
	}
	for key, target := range c.Targets {
		if strings.TrimSpace(key.Service) == "" || strings.TrimSpace(key.Environment) == "" {
			return errors.New("GitHub Actions target service and environment are required")
		}
		if !repositoryPart.MatchString(target.Owner) || !repositoryPart.MatchString(target.Repository) {
			return fmt.Errorf("invalid GitHub repository for %s/%s", key.Service, key.Environment)
		}
		if !workflowFile.MatchString(target.DeployWorkflow) || !workflowFile.MatchString(target.RollbackWorkflow) {
			return fmt.Errorf("workflow must be a .yml or .yaml basename for %s/%s", key.Service, key.Environment)
		}
		if !gitRef.MatchString(target.Ref) || strings.Contains(target.Ref, "..") || strings.HasPrefix(target.Ref, "/") {
			return fmt.Errorf("invalid GitHub ref for %s/%s", key.Service, key.Environment)
		}
	}
	return nil
}
