BINARY_NAME := duplicacy-backup
VERSION     := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME  := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS     := -s -w -X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)
BUILD_DIR   := build

.PHONY: all clean build test fmt vet lint synology

# Default: build for current platform
all: build

build:
	@echo "Building for current platform..."
	go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/duplicacy-backup/

# Cross-compile for all common Synology architectures
synology: synology-amd64 synology-arm64 synology-arm

synology-amd64:
	@echo "Building for Synology linux/amd64 (DS920+, DS1621+, etc.)..."
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
		go build -ldflags "$(LDFLAGS)" \
		-o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 \
		./cmd/duplicacy-backup/

synology-arm64:
	@echo "Building for Synology linux/arm64 (DS223, DS423, etc.)..."
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 \
		go build -ldflags "$(LDFLAGS)" \
		-o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 \
		./cmd/duplicacy-backup/

synology-arm:
	@echo "Building for Synology linux/arm (older ARM models)..."
	GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 \
		go build -ldflags "$(LDFLAGS)" \
		-o $(BUILD_DIR)/$(BINARY_NAME)-linux-armv7 \
		./cmd/duplicacy-backup/

test:
	go test -v -race ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

lint: fmt vet

clean:
	rm -rf $(BUILD_DIR)
