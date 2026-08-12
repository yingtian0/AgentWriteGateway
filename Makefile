.PHONY: run test test-race lint vet check fmt fmt-check

run:
	go run ./cmd/gateway

test:
	go test ./...

test-race:
	go test -race ./...

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
