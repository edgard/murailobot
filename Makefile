GO ?= go
GORELEASER ?= goreleaser
BINARY_NAME ?= murailobot

.PHONY: all build release clean

all: build

build:
	@$(GO) mod tidy && $(GO) mod download
	@$(GO) build -o $(BINARY_NAME) ./...

release: build
	@$(GORELEASER) release --clean

clean:
	@rm -f $(BINARY_NAME)
