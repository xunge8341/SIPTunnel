FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN go build -o gateway ./cmd/gateway

FROM alpine:3.20
WORKDIR /app
COPY --from=builder /app/gateway ./gateway
EXPOSE 18080
CMD ["./gateway"]
