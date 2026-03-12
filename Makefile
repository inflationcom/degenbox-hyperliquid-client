.PHONY: build test lint clean install run-testnet help docker-build docker-run fmt deps dev

VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS=-ldflags "-X main.version=${VERSION} -X main.buildTime=${BUILD_TIME} -s -w"

help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "  build          Build all binaries"
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
	rm -rf bin coverage.out coverage.html

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
