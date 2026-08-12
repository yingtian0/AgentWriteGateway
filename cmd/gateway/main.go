package main

import (
	"flag"
	"log/slog"
	"net/http"
	"os"
	"time"

	"agentwritegateway/internal/api"
	"agentwritegateway/internal/catalog"
	"agentwritegateway/internal/engine"
	"agentwritegateway/internal/executor"
	"agentwritegateway/internal/planner"
	"agentwritegateway/internal/policy"
	"agentwritegateway/internal/store"
)

func main() {
	address := flag.String("addr", ":8080", "HTTP listen address")
	catalogPath := flag.String("catalog", "config/services.json", "service catalog path")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	services, err := catalog.Load(*catalogPath)
	if err != nil {
		logger.Error("load catalog", "error", err)
		os.Exit(1)
	}
	releasePlanner, err := planner.New(services)
	if err != nil {
		logger.Error("initialize planner", "error", err)
		os.Exit(1)
	}

	service := engine.New(releasePlanner, policy.New(), executor.NewMock(nil), store.NewMemory())
	server := &http.Server{
		Addr: *address, Handler: api.New(service, logger).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	logger.Info("gateway listening", "address", *address, "services", len(services), "adapter", "mock", "store", "memory")
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
