# Build variables
GO ?= go
GORELEASER ?= goreleaser
GOLANGCI_LINT ?= golangci-lint
BINARY_NAME ?= murailobot
VERSION ?= $(shell git describe --tags --always --dirty)
COMMIT_HASH ?= $(shell git rev-parse --short HEAD)
BUILD_TIME ?= $(shell date -u '+%Y-%m-%d_%H:%M:%S')

# Go build flags
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.commitHash=$(COMMIT_HASH) -X main.buildTime=$(BUILD_TIME)"

# Cross-compilation targets
PLATFORMS ?= linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

# Docker variables
DOCKER_IMAGE ?= murailobot
DOCKER_TAG ?= latest

# Colors for pretty output
CYAN := \033[36m
GREEN := \033[32m
YELLOW := \033[33m
RED := \033[31m
RESET := \033[0m

.PHONY: all build release clean test vet lint help docker docker-push coverage deps install-deps cross-build dev

# Use .SILENT to reduce noise (remove @ from commands)
.SILENT:

# Default target
all: lint test vet build

# Show help
help: ## Show this help message
	echo "Usage: make [target]"
	echo ""
	echo "Targets:"
	awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  $(CYAN)%-15s$(RESET) %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# Build the application
build: deps ## Build the application
	echo "$(GREEN)Building $(BINARY_NAME)...$(RESET)"
	$(GO) mod tidy && $(GO) mod download
	$(GO) build $(LDFLAGS) -o $(BINARY_NAME) .
	echo "$(GREEN)Build successful!$(RESET)"

# Run tests with coverage
test: ## Run tests
	echo "$(YELLOW)Running tests...$(RESET)"
	$(GO) test -v -race ./...

# Run go vet
vet: ## Run go vet
	echo "$(YELLOW)Running go vet...$(RESET)"
	$(GO) vet ./...

# Run linter
lint: ## Run golangci-lint
	echo "$(YELLOW)Running linter...$(RESET)"
	$(GOLANGCI_LINT) run

# Generate test coverage
coverage: ## Generate test coverage report
	echo "$(YELLOW)Generating coverage report...$(RESET)"
	$(GO) test -race -coverprofile=coverage.out -covermode=atomic ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	echo "$(GREEN)Coverage report generated: coverage.html$(RESET)"

# Check and install dependencies
deps: ## Check and download dependencies
	echo "$(YELLOW)Checking dependencies...$(RESET)"
	$(GO) mod tidy
	$(GO) mod verify
	$(GO) mod download

# Install development tools
install-deps: ## Install development dependencies
	echo "$(YELLOW)Installing development dependencies...$(RESET)"
	$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	$(GO) install github.com/goreleaser/goreleaser@latest

# Cross-platform build
cross-build: deps ## Build for multiple platforms
	echo "$(YELLOW)Building for multiple platforms...$(RESET)"
	$(foreach platform,$(PLATFORMS),\
		echo "$(GREEN)Building for $(platform)...$(RESET)" && \
		GOOS=$(word 1,$(subst /, ,$(platform))) \
		GOARCH=$(word 2,$(subst /, ,$(platform))) \
		$(GO) build $(LDFLAGS) -o $(BINARY_NAME)_$(subst /,_,$(platform))$(if $(findstring windows,$(platform)),.exe,) . || exit 1;)

# Docker targets
docker: ## Build Docker image
	echo "$(YELLOW)Building Docker image...$(RESET)"
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT_HASH=$(COMMIT_HASH) \
		--build-arg BUILD_TIME=$(BUILD_TIME) .

docker-push: docker ## Push Docker image
	echo "$(YELLOW)Pushing Docker image...$(RESET)"
	docker push $(DOCKER_IMAGE):$(DOCKER_TAG)

# Release build
release: lint test vet build ## Create a new release
	echo "$(YELLOW)Creating release...$(RESET)"
	$(GORELEASER) release --clean

# Development environment
dev: install-deps ## Set up development environment
	echo "$(GREEN)Development environment setup complete$(RESET)"

# Clean build artifacts
clean: ## Clean build artifacts
	echo "$(YELLOW)Cleaning build artifacts...$(RESET)"
	rm -f $(BINARY_NAME)*
	rm -f coverage.out coverage.html
	rm -rf dist/
	$(GO) clean -cache -testcache -modcache
	echo "$(GREEN)Cleanup complete$(RESET)"

# Error handling wrapper (usage: $(call check_error))
define check_error
	if [ $$? -ne 0 ]; then \
		echo "$(RED)Error: Command failed$(RESET)"; \
		exit 1; \
	fi
endef
