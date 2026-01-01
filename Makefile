.PHONY: all build test lint clean run benchmark help install-linter escape-analysis migrate

# Variables
PACKAGE := github.com/BullionBear/seq
BINARY_NAME=seq
BIN_DIR=./bin
CMD_DIR=./cmd
COVERAGE_FILE=coverage.out
VERSION := $(shell git describe --tags --always --abbrev=0 --match='v[0-9]*.[0-9]*.[0-9]*' 2> /dev/null || echo "dev")
COMMIT_HASH := $(shell git rev-parse --short HEAD 2> /dev/null || echo "unknown")
BUILD_TIMESTAMP := $(shell date '+%Y-%m-%dT%H:%M:%S')
LDFLAGS := -X '${PACKAGE}/env.Version=${VERSION}' \
           -X '${PACKAGE}/env.CommitHash=${COMMIT_HASH}' \
           -X '${PACKAGE}/env.BuildTime=${BUILD_TIMESTAMP}'

# Default target
.DEFAULT_GOAL := all

# Run all checks: build locally for tests
all:
	@$(MAKE) build-local
	@$(MAKE) test

# Build for local platform (used by test and run)
build-local:
	@echo "Building $(BINARY_NAME) for local platform..."
	@echo "Version: $(VERSION), Commit: $(COMMIT_HASH), BuildTime: $(BUILD_TIMESTAMP)"
	@mkdir -p $(BIN_DIR)
	@go build -ldflags="$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY_NAME) $(CMD_DIR)/main.go
	@echo "Build complete: $(BIN_DIR)/$(BINARY_NAME)"

# Build the application for Linux AMD64
build:
	@echo "Building $(BINARY_NAME) for linux/amd64..."
	@echo "Version: $(VERSION), Commit: $(COMMIT_HASH), BuildTime: $(BUILD_TIMESTAMP)"
	@mkdir -p $(BIN_DIR)
	@env GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY_NAME)-linux-amd64 $(CMD_DIR)/main.go
	@echo "Build complete: $(BIN_DIR)/$(BINARY_NAME)-linux-amd64"

# Run the application (builds for local platform first)
run: build-local
	@echo "Running $(BINARY_NAME)..."
	@$(BIN_DIR)/$(BINARY_NAME)

# Run database migrations
# Usage: make migrate [CONFIG=config/local.yml]
migrate:
	@echo "Running database migrations..."
	@if [ -z "$(CONFIG)" ]; then \
		go run $(CMD_DIR)/migrate/main.go -c config/local.yml; \
	else \
		go run $(CMD_DIR)/migrate/main.go -c $(CONFIG); \
	fi

# Run tests
test:
	@echo "Running tests..."
	@go test -v -race ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	@go test -v -race -coverprofile=$(COVERAGE_FILE) ./...
	@go tool cover -html=$(COVERAGE_FILE) -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Run benchmarks
benchmark:
	@echo "Running benchmarks..."
	@go test -benchmem -bench=. ./...

# Run zero-allocation test
test-zero-alloc:
	@echo "Running zero-allocation test..."
	@go test -v -run TestLogger_ZeroAllocation ./pkg/logger/

# Lint the code using golangci-lint
lint:
	@echo "Running linter..."
	@if ! command -v golangci-lint >/dev/null 2>&1; then \
		echo "golangci-lint not found. Installing..."; \
		$(MAKE) install-linter; \
	fi
	@golangci-lint run --timeout=5m

# Install golangci-lint
install-linter:
	@echo "Installing golangci-lint..."
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf $(BIN_DIR)
	@rm -f $(COVERAGE_FILE) coverage.html escape-analysis.txt
	@go clean
	@echo "Clean complete"

# Verify dependencies
verify:
	@echo "Verifying dependencies..."
	@go mod verify

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	@go mod download
	@go mod tidy

# Format code
fmt:
	@echo "Formatting code..."
	@go fmt ./...

# Vet code
vet:
	@echo "Running go vet..."
	@go vet ./...

# Run escape analysis to see what escapes to heap
escape-analysis:
	@echo "Running escape analysis..."
	@echo "=== Escape Analysis Report ==="
	@go build -gcflags="-m" ./... 2>&1 | grep -E "(escapes|moved to heap|escape analysis)" || go build -gcflags="-m" ./... 2>&1

# Run detailed escape analysis (more verbose)
escape-analysis-detail:
	@echo "Running detailed escape analysis..."
	@echo "=== Detailed Escape Analysis Report ==="
	@go build -gcflags="-m -m" ./... 2>&1 | tee escape-analysis.txt || go build -gcflags="-m -m" ./... 2>&1 | tee escape-analysis.txt

# Show help
help:
	@echo "Available targets:"
	@echo "  make (or make all)  - Build and run all tests (default)"
	@echo "  make build          - Build for linux/amd64 to $(BIN_DIR)/$(BINARY_NAME)-linux-amd64"
	@echo "  make run            - Build for local platform and run the application"
	@echo "  make test           - Run tests"
	@echo "  make test-coverage  - Run tests with coverage report"
	@echo "  make benchmark      - Run benchmarks"
	@echo "  make test-zero-alloc - Run zero-allocation test"
	@echo "  make lint           - Run golangci-lint"
	@echo "  make install-linter - Install golangci-lint"
	@echo "  make clean          - Remove build artifacts"
	@echo "  make verify         - Verify dependencies"
	@echo "  make deps           - Download and tidy dependencies"
	@echo "  make fmt            - Format code"
	@echo "  make vet            - Run go vet"
	@echo "  make escape-analysis - Run escape analysis (shows heap allocations)"
	@echo "  make escape-analysis-detail - Run detailed escape analysis"
	@echo "  make migrate        - Run database migrations (use CONFIG=path/to/config.yml for custom config)"
	@echo "  make help           - Show this help message"

