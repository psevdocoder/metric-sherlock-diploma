FROM golang:1.26.1-alpine AS builder

WORKDIR /build

RUN apk add --no-cache ca-certificates git

COPY CA.crt /usr/local/share/ca-certificates/
RUN update-ca-certificates

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY "cmd" "./cmd"
COPY internal ./internal
COPY migrations ./migrations
COPY pkg ./pkg

RUN go build -o app "./cmd"

FROM alpine:latest

WORKDIR /app

COPY --from=builder /build/app .

COPY migrations ./migrations

EXPOSE 8080

VOLUME ["./values"]

CMD ["./app"]