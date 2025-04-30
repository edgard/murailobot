# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTIDY=$(GOCMD) mod tidy
GORUN=$(GOCMD) run

# Binary name
BINARY_NAME=murailobot
BINARY_UNIX=$(BINARY_NAME)

# Main package path
CMD_PATH=./cmd/bot

# Default target
all: tidy fmt lint build

# Build the application
build:
	@echo "Building $(BINARY_NAME)..."
	$(GOBUILD) -o $(BINARY_UNIX) $(CMD_PATH)
	@echo "$(BINARY_NAME) built successfully."

# Run the application
run:
	@echo "Running $(BINARY_NAME)..."
	$(GORUN) $(CMD_PATH) --config ./config.yaml

# Tidy dependencies
tidy:
	@echo "Tidying dependencies..."
	$(GOTIDY)

# Format the code
fmt:
	@echo "Formatting the code..."
	golangci-lint-v2 fmt ./...

# Lint the code
lint:
	@echo "Linting the code..."
	golangci-lint-v2 run ./...

# Clean build artifacts
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	rm -f $(BINARY_UNIX)

# Phony targets
.PHONY: all build run tidy fmt lint clean
