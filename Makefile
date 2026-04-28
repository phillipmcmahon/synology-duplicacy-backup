BINARY_NAME := duplicacy-backup
VERSION     := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME  := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS     := -s -w -X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)
BUILD_DIR   := build
RELEASE_VERSION ?=

.PHONY: all clean build test fmt vet staticcheck lint validate validate-full synology package-synology package-synology-amd64 package-synology-arm64 package-synology-armv7 release-prep

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

package-synology: package-synology-amd64 package-synology-arm64 package-synology-armv7

package-synology-amd64:
	@echo "Packaging Synology linux/amd64 artifact in Linux container..."
	sh ./scripts/package-linux-docker.sh \
		--version "$(VERSION)" \
		--build-time "$(BUILD_TIME)" \
		--goos linux \
		--goarch amd64 \
		--output-dir /work/build/test-packages/release/$(VERSION)

package-synology-arm64:
	@echo "Packaging Synology linux/arm64 artifact in Linux container..."
	sh ./scripts/package-linux-docker.sh \
		--version "$(VERSION)" \
		--build-time "$(BUILD_TIME)" \
		--goos linux \
		--goarch arm64 \
		--output-dir /work/build/test-packages/release/$(VERSION)

package-synology-armv7:
	@echo "Packaging Synology linux/armv7 artifact in Linux container..."
	sh ./scripts/package-linux-docker.sh \
		--version "$(VERSION)" \
		--build-time "$(BUILD_TIME)" \
		--goos linux \
		--goarch arm \
		--goarm 7 \
		--output-dir /work/build/test-packages/release/$(VERSION)

release-prep:
	@echo "Running strict release-prep flow..."
	sh ./scripts/release-prep.sh $(if $(RELEASE_VERSION),--version "$(RELEASE_VERSION)",)

test:
	go test -v -race ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

staticcheck:
	go run honnef.co/go/tools/cmd/staticcheck ./...

lint: fmt vet staticcheck

validate:
	sh ./scripts/validate-before-push.sh

validate-full:
	sh ./scripts/validate-before-push.sh --with-ui-smoke

clean:
	rm -rf $(BUILD_DIR)
