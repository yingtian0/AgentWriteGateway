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
	return result, nil
}

func healthHandler(configuration settings) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(writer, `{"status":"alive","accepting_actions":false,"runner_id":%q,"runner_group":%q}`, configuration.RunnerID, configuration.RunnerGroup)
	})
	return mux
}
