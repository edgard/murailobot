FROM --platform=linux/arm64 gcr.io/distroless/cc-debian12:latest

WORKDIR /app

# GoReleaser will copy the pre-built binary
COPY murailobot /app/murailobot

ENTRYPOINT ["/app/murailobot"]
