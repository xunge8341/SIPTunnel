FROM --platform=$BUILDPLATFORM golang:1.23-alpine AS builder
WORKDIR /app

ARG TARGETOS
ARG TARGETARCH

COPY go.mod ./
COPY go.sum ./
COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} \
    go build -trimpath -o /out/gateway ./cmd/gateway

FROM alpine:3.20
WORKDIR /app
COPY --from=builder /out/gateway ./gateway
EXPOSE 18080
CMD ["./gateway"]
