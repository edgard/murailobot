GO ?= go
GORELEASER ?= goreleaser
BINARY_NAME ?= murailobot
GOLANGCI_LINT ?= golangci-lint

.PHONY: all build release clean test vet lint

all: lint test vet build

build:
	@$(GO) mod tidy && $(GO) mod download
	@$(GO) build -o $(BINARY_NAME) .

test:
	@echo "Running tests..."
	@$(GO) test -v ./...

vet:
	@echo "Running go vet..."
	@$(GO) vet ./...

lint:
	@echo "Running linter..."
	@$(GOLANGCI_LINT) run

release: lint test vet build
	@$(GORELEASER) release --clean

clean:
	@rm -f $(BINARY_NAME)
