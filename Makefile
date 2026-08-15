.PHONY: run compose-up compose-down test test-race test-integration test-planner benchmark-planner lint vet check fmt fmt-check

run:
	go run ./cmd/gateway

compose-up:
	docker compose -f deploy/compose/compose.yaml up -d

compose-down:
	docker compose -f deploy/compose/compose.yaml down

test:
	go test ./...

test-race:
	go test -race ./...

test-integration:
	docker compose -f deploy/compose/compose.yaml up -d postgres temporal
	AWG_INTEGRATION=1 go test -tags=integration -count=1 ./test/integration

test-planner:
	go test -run TestPlan -count=20 ./internal/planner

benchmark-planner:
	go test -bench . ./internal/planner

lint: vet

vet:
	go vet ./...

check: fmt-check test test-race lint

fmt:
	gofmt -w $$(find . -name '*.go' -type f)

fmt-check:
	@unformatted="$$(gofmt -l $$(find . -name '*.go' -type f))"; \
	if [ -n "$$unformatted" ]; then \
		echo "The following Go files are not formatted:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi
