# Makefile for github.com/zakame/speedtest-go-exporter

BINARY      := speedtest-go-exporter
CMD         := ./cmd/$(BINARY)
BIN_DIR     := ./bin
CGO_ENABLED ?= 0

.DEFAULT_GOAL := help

.PHONY: help build test cover lint vet fmt run clean

help: ## Show this help message
	@echo "Usage: make <target>"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| sort \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'

build: ## Compile the binary to ./bin/speedtest-go-exporter
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=$(CGO_ENABLED) go build -o $(BIN_DIR)/$(BINARY) $(CMD)

test: ## Run all tests
	go test -v -count=1 ./...

cover: ## Run tests with coverage report
	go test -cover -coverpkg=./... -coverprofile=coverage.txt -covermode=atomic ./...
	go tool cover -func=coverage.txt

lint: ## Run golangci-lint if available, otherwise fall back to go vet
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not found, falling back to go vet"; \
		go vet ./...; \
	fi

vet: ## Run go vet
	go vet ./...

fmt: ## Check formatting (exits non-zero if files need formatting)
	@test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)

run: build ## Build and run the exporter (env: SPEEDTEST_PORT, SPEEDTEST_SERVER, SPEEDTEST_EXPORTER_DEBUG)
	$(BIN_DIR)/$(BINARY)

clean: ## Remove build artifacts (./bin/, coverage.txt)
	rm -rf $(BIN_DIR) coverage.txt
