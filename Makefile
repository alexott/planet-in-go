.PHONY: build test clean install fmt lint check build-all help

# Binary name
BINARY=planet

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOINSTALL=$(GOCMD) install
GOCLEAN=$(GOCMD) clean
GOFMT=$(GOCMD) fmt
GOVET=$(GOCMD) vet

# Build flags
LDFLAGS=-ldflags "-s -w"

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-15s %s\n", $$1, $$2}'

build: ## Build the planet binary
	$(GOBUILD) -o $(BINARY) ./cmd/planet

build-release: ## Build with optimizations (smaller binary)
	$(GOBUILD) $(LDFLAGS) -o $(BINARY) ./cmd/planet

test: ## Run all tests
	$(GOTEST) ./... -v

test-coverage: ## Run tests with coverage report
	$(GOTEST) ./... -cover -coverprofile=coverage.out
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

clean: ## Clean build artifacts and test caches
	$(GOCLEAN)
	rm -f $(BINARY)
	rm -f $(BINARY)-*
	rm -f coverage.out coverage.html
	rm -rf ./test-cache ./test-output
	rm -rf ./cache ./output

install: ## Install the binary to $GOPATH/bin
	$(GOINSTALL) ./cmd/planet

fmt: ## Format Go code
	$(GOFMT) ./...

lint: ## Run go vet
	$(GOVET) ./...

check: fmt lint test ## Run all checks (fmt, lint, test)

build-all: ## Build for multiple platforms
	@echo "Building for Linux (amd64)..."
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BINARY)-linux-amd64 ./cmd/planet
	@echo "Building for macOS (amd64)..."
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BINARY)-darwin-amd64 ./cmd/planet
	@echo "Building for macOS (arm64)..."
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BINARY)-darwin-arm64 ./cmd/planet
	@echo "Building for Windows (amd64)..."
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BINARY)-windows-amd64.exe ./cmd/planet
	@echo "Done! Binaries created:"
	@ls -lh $(BINARY)-*

run: build ## Build and run with default config
	./$(BINARY) -c config.ini

run-example: build ## Build and run with example config
	@echo "Creating example config..."
	@mkdir -p example-cache example-output
	@echo '[Planet]\nname = Example Planet\nlink = http://example.com\ncache_directory = ./example-cache\noutput_dir = ./example-output\nlog_level = INFO\nfeed_timeout = 20\nitems_per_page = 10\ntemplate_files = examples/simple-template.html.tmpl\n\n[https://go.dev/blog/feed.atom]\nname = Go Blog' > example-config.ini
	./$(BINARY) -c example-config.ini
	@echo "\nOutput generated in example-output/"

deps: ## Download dependencies
	$(GOCMD) mod download
	$(GOCMD) mod tidy

upgrade-deps: ## Upgrade all dependencies
	$(GOCMD) get -u ./...
	$(GOCMD) mod tidy

size: build-release ## Show binary size
	@ls -lh $(BINARY) | awk '{print "Binary size: " $$5}'
	@if command -v du >/dev/null 2>&1; then \
		echo "Disk usage: $$(du -h $(BINARY) | cut -f1)"; \
	fi

version: build ## Show version
	./$(BINARY) -version

# Development helpers
dev-test: ## Run tests in watch mode (requires entr)
	@which entr > /dev/null || (echo "entr not installed. Install with: brew install entr" && exit 1)
	@echo "Watching for changes... (Ctrl+C to stop)"
	@find . -name '*.go' | entr -c make test

dev-run: ## Run in development mode with example config
	@make run-example

# Docker targets (optional)
docker-build: ## Build Docker image
	docker build -t planet-go:latest .

docker-run: ## Run in Docker container
	docker run --rm -v $(PWD)/config.ini:/config.ini planet-go:latest

# Benchmark targets
bench: ## Run benchmarks
	$(GOTEST) -bench=. -benchmem ./...

bench-compare: ## Run benchmarks and save for comparison
	$(GOTEST) -bench=. -benchmem ./... | tee bench.txt
	@echo "Benchmark results saved to bench.txt"

# Profiling targets
profile-cpu: ## Generate CPU profile
	$(GOTEST) -cpuprofile=cpu.prof -bench=. ./...
	$(GOCMD) tool pprof -http=:8080 cpu.prof

profile-mem: ## Generate memory profile
	$(GOTEST) -memprofile=mem.prof -bench=. ./...
	$(GOCMD) tool pprof -http=:8080 mem.prof

.DEFAULT_GOAL := help

