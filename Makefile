# Build variables
BINARY_NAME=murailobot
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
BUILT_BY=makefile
LDFLAGS=-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE} -X main.builtBy=${BUILT_BY}
MAIN_PATH=./cmd/murailobot

.PHONY: all default build clean test lint vet mod generate help

default: all

all: mod generate lint vet test build

help:
	@echo "Available commands:"
	@echo "  all           - Run mod, generate, lint, vet, tests and build the binary (default)"
	@echo "  build         - Build the binary"
	@echo "  clean         - Remove build artifacts"
	@echo "  test          - Run tests"
	@echo "  lint          - Run golangci-lint"
	@echo "  vet           - Run go vet"
	@echo "  mod           - Run go mod tidy and download"
	@echo "  generate      - Run go generate"

build:
	CGO_ENABLED=1 go build -ldflags "${LDFLAGS}" -o ${BINARY_NAME} ${MAIN_PATH}

clean:
	rm -f ${BINARY_NAME}
	rm -rf dist/

test:
	go test ./...

lint:
	golangci-lint run

vet:
	go vet ./...

mod:
	go mod tidy
	go mod download

generate:
	go generate ./...
