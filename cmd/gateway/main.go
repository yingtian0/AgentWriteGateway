package main

import (
	"flag"
	"log/slog"
	"net/http"
	"os"
	"time"

	"agentwritegateway/internal/api"
	"agentwritegateway/internal/catalog"
	"agentwritegateway/internal/contract"
	"agentwritegateway/internal/engine"
	"agentwritegateway/internal/executor"
	"agentwritegateway/internal/planner"
	"agentwritegateway/internal/policy"
	"agentwritegateway/internal/profile"
	"agentwritegateway/internal/store"
)

func main() {
	address := flag.String("addr", ":8080", "HTTP listen address")
	contractPath := flag.String("contracts", "examples/contracts", "service contract directory")
	profilePath := flag.String("profiles", "examples/profiles", "release profile directory")
	catalogPath := flag.String("catalog", "", "legacy JSON service catalog path (compatibility mode)")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	contracts, profiles, err := loadPlanningInputs(*contractPath, *profilePath, *catalogPath)
	if err != nil {
		logger.Error("load planning inputs", "error", err)
		os.Exit(1)
	}
	releasePlanner, err := planner.NewFromContracts(contracts, profiles, planner.Options{})
	if err != nil {
		logger.Error("initialize contract planner", "error", err)
		os.Exit(1)
	}

	service := engine.New(releasePlanner, policy.New(), executor.NewMock(nil), store.NewMemory())
	server := &http.Server{
		Addr: *address, Handler: api.New(service, logger).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	logger.Info("gateway listening", "address", *address, "services", len(contracts), "adapter", "mock", "store", "memory")
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func loadPlanningInputs(contractPath, profilePath, catalogPath string) ([]contract.ServiceContract, []profile.ReleaseProfile, error) {
	if catalogPath != "" {
		return catalog.LoadContracts(catalogPath)
	}
	contracts, err := contract.LoadDir(contractPath)
	if err != nil {
		return nil, nil, err
	}
	profiles, err := profile.LoadDir(profilePath)
	if err != nil {
		return nil, nil, err
	}
	return contracts, profiles, nil
}
