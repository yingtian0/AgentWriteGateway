package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"agentwritegateway/adapters/datadog"
	"agentwritegateway/adapters/githubactions"
	"agentwritegateway/pkg/protocol"

	"go.yaml.in/yaml/v3"
)

type settings struct {
	Mode          string                `yaml:"mode"`
	RunnerID      string                `yaml:"runner_id"`
	RunnerGroup   string                `yaml:"runner_group"`
	TenantID      string                `yaml:"tenant_id"`
	HealthAddress string                `yaml:"health_address"`
	ControlPlane  controlPlaneSettings  `yaml:"control_plane"`
	Identity      identitySettings      `yaml:"identity"`
	Policy        policySettings        `yaml:"policy"`
	Journal       journalSettings       `yaml:"journal"`
	Adapters      adapterSettings       `yaml:"adapters"`
	Credentials   credentialSettings    `yaml:"credentials"`
	Capabilities  []protocol.Capability `yaml:"capabilities"`
}
type controlPlaneSettings struct {
	Address      string `yaml:"address"`
	Issuer       string `yaml:"issuer"`
	TrustKeyFile string `yaml:"trust_key_file"`
}
type identitySettings struct {
	Issuer       string `yaml:"issuer"`
	Audience     string `yaml:"audience"`
	TrustKeyFile string `yaml:"trust_key_file"`
}
type policySettings struct {
	BundleFile   string `yaml:"bundle_file"`
	TrustKeyFile string `yaml:"trust_key_file"`
}
type journalSettings struct {
	DatabaseURL string `yaml:"database_url"`
}
type adapterSettings struct {
	GitHubActions githubActionsSettings `yaml:"github_actions"`
	Datadog       datadogSettings       `yaml:"datadog"`
}
type githubActionsSettings struct {
	Targets []githubTargetSettings `yaml:"targets"`
}
type githubTargetSettings struct {
	Service          string `yaml:"service"`
	Environment      string `yaml:"environment"`
	Owner            string `yaml:"owner"`
	Repository       string `yaml:"repository"`
	DeployWorkflow   string `yaml:"deploy_workflow"`
	RollbackWorkflow string `yaml:"rollback_workflow"`
	Ref              string `yaml:"ref"`
}
type datadogSettings struct {
	Site    datadog.Site           `yaml:"site"`
	Queries []datadogQuerySettings `yaml:"queries"`
}
type datadogQuerySettings struct {
	Service       string  `yaml:"service"`
	Environment   string  `yaml:"environment"`
	Expression    string  `yaml:"expression"`
	Comparator    string  `yaml:"comparator"`
	Threshold     float64 `yaml:"threshold"`
	Aggregation   string  `yaml:"aggregation"`
	MinimumPoints int     `yaml:"minimum_points"`
	MaximumAge    string  `yaml:"maximum_age"`
}
type credentialSettings struct {
	GitHubTokenFile       string `yaml:"github_token_file"`
	DatadogCredentialFile string `yaml:"datadog_credential_file"`
}

func main() {
	configPath := flag.String("config", "config/runner.example.yaml", "runner configuration path")
	check := flag.Bool("check-config", false, "validate configuration and exit")
	flag.Parse()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	configuration, err := load(*configPath)
	if err != nil {
		logger.Error("load runner config", "error", err)
		os.Exit(1)
	}
	if *check {
		logger.Info("runner configuration valid", "runner_id", configuration.RunnerID, "runner_group", configuration.RunnerGroup)
		return
	}
	server := &http.Server{Addr: configuration.HealthAddress, Handler: healthHandler(configuration), ReadHeaderTimeout: 2 * time.Second}
	go func() {
		logger.Info("runner health endpoint listening", "address", configuration.HealthAddress, "runner_group", configuration.RunnerGroup)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("runner health server", "error", err)
		}
	}()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdown)
}

func load(path string) (settings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return settings{}, err
	}
	data = []byte(os.ExpandEnv(string(data)))
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	decoder.KnownFields(true)
	var result settings
	if err := decoder.Decode(&result); err != nil {
		return settings{}, err
	}
	if result.Mode != "development" && result.Mode != "production" {
		return settings{}, fmt.Errorf("mode must be development or production")
	}
	if result.RunnerID == "" || result.RunnerGroup == "" || result.TenantID == "" || result.ControlPlane.Address == "" || result.ControlPlane.Issuer == "" || result.Identity.Issuer == "" || result.Identity.Audience == "" || result.Policy.BundleFile == "" || len(result.Capabilities) == 0 {
		return settings{}, fmt.Errorf("runner identity, trust endpoints, policy bundle, and capabilities are required")
	}
	if result.HealthAddress == "" {
		result.HealthAddress = ":8081"
	}
	for _, capability := range result.Capabilities {
		if capability != protocol.CapabilityDeploy && capability != protocol.CapabilityRollback {
			return settings{}, fmt.Errorf("unsupported capability %q", capability)
		}
	}
	if result.Mode == "production" && (result.Journal.DatabaseURL == "" || result.ControlPlane.TrustKeyFile == "" || result.Identity.TrustKeyFile == "" || result.Policy.TrustKeyFile == "") {
		return settings{}, fmt.Errorf("production requires durable journal and customer-managed grant, OIDC, and policy trust keys")
	}
	if result.Mode == "production" && !strings.HasPrefix(result.ControlPlane.Address, "https://") {
		return settings{}, fmt.Errorf("production control plane address must use https")
	}
	if err := validateAdapters(result); err != nil {
		return settings{}, err
	}
	return result, nil
}

func validateAdapters(configuration settings) error {
	githubTargets := make(map[githubactions.TargetKey]githubactions.Target, len(configuration.Adapters.GitHubActions.Targets))
	for _, target := range configuration.Adapters.GitHubActions.Targets {
		key := githubactions.TargetKey{Service: target.Service, Environment: target.Environment}
		if _, duplicate := githubTargets[key]; duplicate {
			return fmt.Errorf("duplicate GitHub Actions target %s/%s", key.Service, key.Environment)
		}
		githubTargets[key] = githubactions.Target{Owner: target.Owner, Repository: target.Repository, DeployWorkflow: target.DeployWorkflow, RollbackWorkflow: target.RollbackWorkflow, Ref: target.Ref}
	}
	if len(githubTargets) > 0 {
		if _, err := githubactions.New(githubactions.Config{Targets: githubTargets}, nil); err != nil {
			return fmt.Errorf("GitHub Actions adapter: %w", err)
		}
	}
	datadogQueries := make(map[datadog.QueryKey]datadog.Query, len(configuration.Adapters.Datadog.Queries))
	for _, configured := range configuration.Adapters.Datadog.Queries {
		maximumAge, err := time.ParseDuration(configured.MaximumAge)
		if err != nil {
			return fmt.Errorf("Datadog maximum_age for %s/%s: %w", configured.Service, configured.Environment, err)
		}
		key := datadog.QueryKey{Service: configured.Service, Environment: configured.Environment}
		if _, duplicate := datadogQueries[key]; duplicate {
			return fmt.Errorf("duplicate Datadog query %s/%s", key.Service, key.Environment)
		}
		datadogQueries[key] = datadog.Query{Expression: configured.Expression, Comparator: configured.Comparator, Threshold: configured.Threshold, Aggregation: configured.Aggregation, MinimumPoints: configured.MinimumPoints, MaximumAge: maximumAge}
	}
	if len(datadogQueries) > 0 {
		if _, err := datadog.New(datadog.Config{Site: configuration.Adapters.Datadog.Site, Queries: datadogQueries}, nil); err != nil {
			return fmt.Errorf("Datadog adapter: %w", err)
		}
	}
	if configuration.Mode == "production" {
		if len(githubTargets) == 0 || configuration.Credentials.GitHubTokenFile == "" {
			return fmt.Errorf("production deploy requires allow-listed GitHub Actions targets and a Runner-local token file")
		}
		if len(datadogQueries) == 0 || configuration.Credentials.DatadogCredentialFile == "" {
			return fmt.Errorf("production verification requires allow-listed Datadog queries and a Runner-local credential file")
		}
	}
	return nil
}

func healthHandler(configuration settings) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(writer, `{"status":"alive","accepting_actions":false,"deploy_adapter":"github-actions","verification_adapter":"datadog","runner_id":%q,"runner_group":%q}`, configuration.RunnerID, configuration.RunnerGroup)
	})
	return mux
}
