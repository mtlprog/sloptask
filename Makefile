.PHONY: help build run serve check-deadlines clean test lint

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-20s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the application
	@echo "Building sloptask..."
	go build -o bin/sloptask ./cmd/sloptask

run: build ## Build and run the serve command
	./bin/sloptask serve

serve: build ## Build and start the HTTP server
	./bin/sloptask serve

check-deadlines: build ## Build and run the deadline checker
	./bin/sloptask check-deadlines

clean: ## Remove build artifacts
	rm -rf bin/

test: ## Run tests
	go test -v ./...

lint: ## Run linter
	golangci-lint run

deps: ## Download and tidy dependencies
	go mod download
	go mod tidy

.DEFAULT_GOAL := help
