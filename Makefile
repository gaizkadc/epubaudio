GO ?= $(shell which go 2>/dev/null || echo go)
BINARY_NAME=epubaudio
CMD_DIR=./cmd/epubaudio

.PHONY: all help build install run test test-race coverage lint fmt tidy clean

all: build

help: ## Display available targets with descriptions
	@echo "Available Makefile commands:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

build: ## Build the epubaudio binary
	@echo "==> Building $(BINARY_NAME)..."
	$(GO) build -o $(BINARY_NAME) $(CMD_DIR)
	@echo "Done: ./$(BINARY_NAME)"

install: ## Install binary to GOBIN
	@echo "==> Installing $(BINARY_NAME)..."
	$(GO) install $(CMD_DIR)

run: build ## Build and run interactive mode with arguments (e.g. make run ARGS="book.epub")
	./$(BINARY_NAME) $(ARGS)

test: ## Run unit tests
	@echo "==> Running tests..."
	$(GO) test -v ./...

test-race: ## Run unit tests with race detector
	@echo "==> Running tests with race detector..."
	$(GO) test -v -race ./...

coverage: ## Run tests and display coverage report
	@echo "==> Running tests with coverage..."
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out
	@echo "To view HTML report, run: $(GO) tool cover -html=coverage.out"

lint: ## Run static analysis (go vet and golangci-lint if installed)
	@echo "==> Running go vet..."
	$(GO) vet ./...
	@if command -v golangci-lint >/dev/null 2>&1; then \
		echo "==> Running golangci-lint..."; \
		golangci-lint run ./... || true; \
	fi

fmt: ## Format all Go source files
	@echo "==> Formatting code..."
	gofmt -s -w .

tidy: ## Tidy and verify Go module dependencies
	@echo "==> Tidying go.mod and go.sum..."
	$(GO) mod tidy

clean: ## Remove build outputs, coverage reports, and temporary files
	@echo "==> Cleaning build and temporary artifacts..."
	rm -f $(BINARY_NAME) coverage.out coverage.html
	rm -rf temp_audio/ *_audio/ ffmpeg_concat*.txt ffmpeg_metadata*.txt inputs.txt
	find output/ -mindepth 1 -not -name '.gitkeep' -exec rm -rf {} + 2>/dev/null || true
	@echo "Clean completed."
