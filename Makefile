.PHONY: build test bench docker-up docker-down run help

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOMOD=$(GOCMD) mod

# Binary output directory
BIN_DIR=bin
SERVER_BINARY=$(BIN_DIR)/shortlink

# Test packages
TEST_PACKAGES=./...
BENCH_PACKAGE=./internal/service/...

# Docker compose
DC=docker-compose
DC_FILE=-f docker-compose.yaml

help: ## Display this help message
	@echo "ShortLink Makefile Commands:"
	@echo ""
	@echo "  make build       - Build the server binary to bin/ directory"
	@echo "  make run         - Run the server directly (go run)"
	@echo "  make test        - Run all unit tests"
	@echo "  make bench       - Run benchmarks and save to bench_result.txt"
	@echo "  make docker-up   - Start MySQL and Redis containers"
	@echo "  make docker-down - Stop and remove containers"
	@echo "  make clean       - Remove binary and cache files"
	@echo "  make help        - Show this help message"
	@echo ""

build: ## Build server binary to bin/shortlink
	@echo "Building server..."
	@mkdir -p $(BIN_DIR)
	$(GOBUILD) -o $(SERVER_BINARY) ./cmd/server/main.go
	@echo "Binary built: $(SERVER_BINARY)"

run: ## Run the server directly
	@echo "Starting server..."
	$(GOCMD) run ./cmd/server/main.go

test: ## Run all unit tests
	@echo "Running tests..."
	$(GOTEST) -v -cover $(TEST_PACKAGES)

bench: ## Run benchmarks and save results
	@echo "Running benchmarks..."
	$(GOTEST) -bench=. -benchmem -run=none ./internal/service/... 2>&1 | tee bench_result.txt
	@echo "Results saved to bench_result.txt"

docker-up: ## Start MySQL and Redis containers
	@echo "Starting Docker containers..."
	$(DC) $(DC_FILE) up -d
	@echo "Waiting for containers to be healthy..."
	@sleep 5
	$(DC) $(DC_FILE) ps

docker-down: ## Stop and remove containers
	@echo "Stopping Docker containers..."
	$(DC) $(DC_FILE) down

clean: ## Remove binary and cache files
	@echo "Cleaning..."
	@rm -rf $(BIN_DIR)
	@rm -f bench_result.txt
	@go clean -cache

lint: ## Run linter (if installed)
	@which golangci-lint > /dev/null || echo "golangci-lint not found, skipping..."
	@which golangci-lint > /dev/null && golangci-lint run ./... || true

tidy: ## Tidy go modules
	$(GOMOD) tidy
	$(GOMOD) verify