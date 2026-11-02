# Prokolpo API tasks. Requires Go, golangci-lint v2.6+ and Docker for the local database.
BINARY  := api
PKG     := ./cmd/api
LDFLAGS := -s -w

.PHONY: all build run migrate test test-integration cover lint fmt vuln tidy check db-up db-down clean help

all: check build ## Lint, test, then build

build: ## Build the API binary into ./bin
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY) $(PKG)

run: ## Run the API locally (reads .env)
	go run $(PKG)

migrate: ## Apply pending database migrations
	go run ./cmd/migrate up

test: ## Unit tests with the race detector
	go test -race -shuffle=on ./...

test-integration: ## Integration tests against TEST_DATABASE_URL
	go test -race -tags=integration -count=1 ./...

cover: ## Coverage report
	go test -race -covermode=atomic -coverprofile=cover.out ./...
	go tool cover -func=cover.out | tail -1

lint: ## golangci-lint with the repo config
	golangci-lint run ./...

fmt: ## Format code
	golangci-lint fmt ./...

vuln: ## Known-vulnerability scan of reachable code
	go run golang.org/x/vuln/cmd/govulncheck@v1 ./...

tidy: ## Tidy and verify modules
	go mod tidy
	go mod verify

check: ## Everything CI runs
	go mod tidy -diff
	go vet ./...
	$(MAKE) lint test vuln

db-up: ## Start the local Postgres
	docker compose up -d db

db-down: ## Stop the local Postgres (data volume is kept)
	docker compose down

clean: ## Remove build output
	rm -rf bin cover.out

help: ## List targets
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-18s %s\n", $$1, $$2}'
