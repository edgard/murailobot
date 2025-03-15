# Build stage
FROM golang:1.24.1-alpine3.21 AS builder
ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown
WORKDIR /usr/src/app

# Install standard build dependencies
RUN apk add --no-cache build-base

# Copy only what's needed for go mod download first (for better caching)
COPY go.mod go.sum ./
RUN go mod download

# Now copy source and build
COPY . .
RUN CGO_ENABLED=1 go build \
    -o /usr/local/bin/murailobot \
    -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE} -X main.builtBy=docker"

# Final minimal runtime image
FROM alpine:3.21
WORKDIR /app

# Create non-root user for security
RUN adduser -D -u 10001 murailobot

# Copy only the executable from builder stage
COPY --from=builder /usr/local/bin/murailobot /app/

# Use non-root user
USER murailobot

# Define entrypoint
ENTRYPOINT ["/app/murailobot"]
