FROM golang:1.26.1-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG SERVICE
ARG BINARY_NAME=service
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/${BINARY_NAME} ./cmd/${SERVICE}
FROM alpine:3.21
WORKDIR /app
RUN apk --no-cache add ca-certificates tzdata netcat-openbsd
COPY --from=builder /out/${BINARY_NAME} ./service
COPY --from=builder /app/migrations /app/migrations
ENTRYPOINT ["./service"]
