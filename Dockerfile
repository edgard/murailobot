FROM gcr.io/distroless/base:latest
LABEL maintainer="Edgard Castro <castro@edgard.org>"
COPY murailobot /app/
WORKDIR /app
ENTRYPOINT ["/app/murailobot"]
