FROM alpine:latest AS build
RUN apk add --no-cache musl-dev go
WORKDIR /go/src/murailobot
COPY . .
RUN go build -o /go/bin/murailobot .

FROM alpine:latest AS bin
LABEL maintainer="Edgard Castro <castro@edgard.org>"
LABEL org.opencontainers.image.source="https://github.com/edgard/murailobot"
RUN apk add --no-cache ca-certificates
COPY --from=build /go/bin/murailobot /app/murailobot
WORKDIR /app
ENTRYPOINT ["/app/murailobot"]
