.PHONY: build test lint clean run benchmark help install-linter

# Variables
BINARY_NAME=seq
BIN_DIR=./bin
CMD_DIR=./cmd
COVERAGE_FILE=coverage.out

# Default target
.DEFAULT_GOAL := help

# Build the application
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BIN_DIR)
	@go build -o $(BIN_DIR)/$(BINARY_NAME) $(CMD_DIR)/main.go
	@echo "Build complete: $(BIN_DIR)/$(BINARY_NAME)"

# Run the application
run: build
	@echo "Running $(BINARY_NAME)..."
	@$(BIN_DIR)/$(BINARY_NAME)

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
	@rm -f $(COVERAGE_FILE) coverage.html
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

# Show help
help:
	@echo "Available targets:"
	@echo "  make build          - Build the executable to $(BIN_DIR)/$(BINARY_NAME)"
	@echo "  make run            - Build and run the application"
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
	@echo "  make help           - Show this help message"

