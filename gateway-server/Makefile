.PHONY: run test build build-linux-amd64 build-linux-arm64 docker-buildx release-local gen-config-docs

BINARY_NAME ?= gateway
DIST_DIR ?= ../dist/bin
IMAGE ?= siptunnel/gateway-server
TAG ?= dev
PLATFORMS ?= linux/amd64,linux/arm64
PUSH ?= false
OUTPUT ?= type=oci,dest=../dist/images/gateway-$(TAG).tar

run:
	go run ./cmd/gateway

test:
	go test ./...

build:
	go build ./cmd/gateway

build-linux-amd64:
	mkdir -p $(DIST_DIR)/linux/amd64
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o $(DIST_DIR)/linux/amd64/$(BINARY_NAME) ./cmd/gateway

build-linux-arm64:
	mkdir -p $(DIST_DIR)/linux/arm64
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -o $(DIST_DIR)/linux/arm64/$(BINARY_NAME) ./cmd/gateway

docker-buildx:
	mkdir -p ../dist/images
	docker buildx build --platform $(PLATFORMS) -t $(IMAGE):$(TAG) --build-arg TARGETOS=linux $(if $(filter true,$(PUSH)),--push,--output $(OUTPUT)) .

release-local: build-linux-amd64 build-linux-arm64
	@echo "Local release artifacts are available under $(DIST_DIR)/linux/{amd64,arm64}/$(BINARY_NAME)"

gen-config-docs:
	go run ./cmd/configdocgen
