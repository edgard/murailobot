FROM golang:latest AS build
WORKDIR /go/src/murailobot
COPY . .
RUN go build -o /go/bin/murailobot .

FROM ubuntu:latest AS bin
LABEL maintainer="Edgard Castro <castro@edgard.org>"
LABEL org.opencontainers.image.source="https://github.com/edgard/murailobot"
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /go/bin/murailobot /app/murailobot
WORKDIR /app
ENTRYPOINT ["/app/murailobot"]
