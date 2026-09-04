package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"time"

	"themisy/internal/api"
	"themisy/internal/application"
	"themisy/internal/catalog"
	"themisy/internal/config"
	"themisy/internal/contract"
	"themisy/internal/executor"
	"themisy/internal/mcp"
	"themisy/internal/planner"
	"themisy/internal/policy"
	"themisy/internal/profile"
	"themisy/internal/scheduler"
	postgresstore "themisy/internal/store/postgres"
	"themisy/internal/ui"
	workflowcore "themisy/internal/workflow"

	"go.temporal.io/sdk/client"
)

func main() {
	configPath := flag.String("config", "config/themisy.example.yaml", "Themisy configuration path")
	modeOverride := flag.String("mode", "", "control, worker, or all")
	contractOverride := flag.String("contracts", "", "service contract directory override")
	profileOverride := flag.String("profiles", "", "release profile directory override")
	catalogOverride := flag.String("catalog", "", "legacy JSON service catalog compatibility override")
	flag.Parse()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	settings, err := config.Load(*configPath)
	if err != nil {
		logger.Error("load config", "error", err)
		os.Exit(1)
	}
	if *modeOverride != "" {
		settings.Mode = *modeOverride
	}
	if *contractOverride != "" {
		settings.Planning.Contracts = *contractOverride
	}
	if *profileOverride != "" {
		settings.Planning.Profiles = *profileOverride
	}
	if *catalogOverride != "" {
		settings.Planning.LegacyCatalog = *catalogOverride
	}
	if err := settings.Validate(); err != nil {
		logger.Error("validate config", "error", err)
		os.Exit(1)
	}
	ctx := context.Background()
	if settings.Database.AutoMigrate {
		if err := postgresstore.Migrate(settings.Database.URL, false); err != nil {
			logger.Error("migrate postgres", "error", err)
			os.Exit(1)
		}
	}
	persistentStore, err := postgresstore.New(ctx, settings.Database.URL)
	if err != nil {
		logger.Error("open postgres", "error", err)
		os.Exit(1)
	}
	defer persistentStore.Close()
	temporalClient, err := client.Dial(client.Options{HostPort: settings.Temporal.Address, Namespace: settings.Temporal.Namespace})
	if err != nil {
		logger.Error("connect temporal", "error", err)
		os.Exit(1)
	}
	defer temporalClient.Close()
	contracts, profiles, err := loadPlanningInputs(settings.Planning.Contracts, settings.Planning.Profiles, settings.Planning.LegacyCatalog)
	if err != nil {
		logger.Error("load planning inputs", "error", err)
		os.Exit(1)
	}
	policyEngine, err := policy.NewMandatoryEngine(ctx)
	if err != nil {
		logger.Error("initialize mandatory OPA policy", "error", err)
		os.Exit(1)
	}
	releasePlanner, err := planner.NewFromContracts(contracts, profiles, planner.Options{PolicyHash: policyEngine.PolicyHash()})
	if err != nil {
		logger.Error("initialize planner", "error", err)
		os.Exit(1)
	}
	controller := workflowcore.NewController(temporalClient, settings.Temporal.TaskQueue)
	releases := application.NewReleases(releasePlanner, persistentStore, controller)
	go releases.RunWorkflowRecovery(ctx, 5*time.Second, func(err error) { logger.Error("recover workflow outbox", "error", err) })
	activities := workflowcore.NewActivities(persistentStore, policyEngine, executor.NewMock(nil))
	activities.Capacity = func(input workflowcore.ScheduleInput) scheduler.Capacity {
		return scheduler.Capacity{RunnerAvailable: releases.RunnerCapacity(input.Step.Tenant, input.Step.RunnerGroup), AdapterRemaining: 20, QueueLimit: 100}
	}
	if settings.Mode == "worker" {
		runWorker(logger, temporalClient, settings.Temporal.TaskQueue, activities)
		return
	}
	if settings.Mode == "all" {
		worker := workflowcore.NewWorker(temporalClient, settings.Temporal.TaskQueue, activities)
		if err := worker.Start(); err != nil {
			logger.Error("start temporal worker", "error", err)
			os.Exit(1)
		}
		defer worker.Stop()
	}
	uiServer, err := ui.New(releases, ui.HeaderIdentityVerifier{})
	if err != nil {
		logger.Error("initialize status UI", "error", err)
		os.Exit(1)
	}
	routes := http.NewServeMux()
	routes.Handle("/mcp", mcp.NewHTTP(releases, mcp.HeaderPrincipalResolver{}, nil))
	routes.Handle("/ui/", uiServer.Handler())
	routes.Handle("/", api.New(releases, logger).Handler())
	server := &http.Server{Addr: settings.HTTP.Address, Handler: routes, ReadHeaderTimeout: 5 * time.Second}
	logger.Info("Themisy control plane listening", "address", settings.HTTP.Address, "services", len(contracts), "workflow", "temporal", "store", "postgres")
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func runWorker(logger *slog.Logger, temporalClient client.Client, taskQueue string, activities *workflowcore.Activities) {
	logger.Info("temporal worker starting", "task_queue", taskQueue)
	if err := workflowcore.RunWorker(context.Background(), temporalClient, taskQueue, activities); err != nil {
		logger.Error("temporal worker stopped", "error", err)
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
