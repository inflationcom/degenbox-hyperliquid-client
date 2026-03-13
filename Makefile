.PHONY: build build-all checksums test lint clean install run-testnet help docker-build docker-run fmt deps dev

VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS=-ldflags "-X main.version=${VERSION} -X main.buildTime=${BUILD_TIME} -s -w"

PLATFORMS=linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "  build          Build binaries for current platform"
	@echo "  build-all      Cross-compile for all platforms"
	@echo "  checksums      Generate SHA256 checksums for dist/"
	@echo "  test           Run tests"
	@echo "  test-coverage  Run tests with coverage report"
	@echo "  lint           Run golangci-lint"
	@echo "  clean          Remove build artifacts"
	@echo "  install        Install to /usr/local/bin"
	@echo "  run-testnet    Run with testnet config"
	@echo "  docker-build   Build Docker image"
	@echo "  docker-run     Run Docker container"
	@echo "  deps           Download dependencies"
	@echo "  fmt            Format code"

build:
	@mkdir -p bin
	go build $(LDFLAGS) -o bin/bot ./cmd/bot
	go build $(LDFLAGS) -o bin/healthcheck ./cmd/healthcheck

build-all:
	@mkdir -p dist
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; \
		arch=$${platform#*/}; \
		ext=""; \
		if [ "$$os" = "windows" ]; then ext=".exe"; fi; \
		echo "Building $$os/$$arch..."; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build $(LDFLAGS) -o dist/bot-$$os-$$arch$$ext ./cmd/bot; \
	done
	@echo "Done. Binaries in dist/"

checksums:
	@cd dist && shasum -a 256 bot-* > sha256sums.txt
	@echo "Checksums written to dist/sha256sums.txt"

test:
	go test -race ./...

test-coverage:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

lint:
	golangci-lint run ./...

deps:
	go mod download && go mod tidy

fmt:
	go fmt ./...

install: build
	sudo cp bin/bot /usr/local/bin/bot

clean:
	rm -rf bin dist coverage.out coverage.html

run-testnet:
	go run ./cmd/bot --testnet

docker-build:
	docker build -t bot .

docker-run:
	docker run -d \
		--name bot \
		--env-file .env \
		-v $(PWD)/config.json:/app/config.json:ro \
		--restart unless-stopped \
		bot

dev: fmt test
