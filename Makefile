# nota - AI transcript auditing tool
BINARY_NAME=nota
VERSION?=0.1.0
BUILD_DIR=dist

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Build flags
LDFLAGS=-ldflags "-X main.version=$(VERSION)"

.PHONY: all build build-all clean test deps lint install help serve extract demo

# Default target - show help
all: help

# Show help with usage examples
help:
	@echo ""
	@echo "  nota - AI transcript auditing tool"
	@echo "  ====================================="
	@echo ""
	@echo "  Build targets:"
	@echo "    make build       Build for current platform"
	@echo "    make build-all   Build for macOS and Linux (amd64, arm64)"
	@echo "    make clean       Remove build artifacts"
	@echo "    make deps        Download and tidy dependencies"
	@echo "    make test        Run tests"
	@echo "    make lint        Run linter (requires golangci-lint)"
	@echo "    make install     Install to GOPATH/bin"
	@echo ""
	@echo "  Development shortcuts:"
	@echo "    make run         Build and show help"
	@echo "    make serve       Build and start web viewer on :8080"
	@echo "    make extract     Build and extract sessions from current repo"
	@echo "    make extract-all Build and extract all sessions"
	@echo "    make demo        Run a quick demo of all features"
	@echo ""
	@echo "  Quick start:"
	@echo "    1. make build        # Build the binary"
	@echo "    2. ./nota extract    # See AI sessions for this repo"
	@echo "    3. ./nota serve      # Open http://localhost:8080"
	@echo ""

# Build for current platform
build:
	@echo "Building $(BINARY_NAME)..."
	@$(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME) .
	@echo "Done! Run ./$(BINARY_NAME) --help to get started"

# Build for all target platforms (macOS + Linux)
build-all: clean
	@echo "Building for all platforms..."
	@mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 .
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 .
	GOOS=linux GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 .
	@echo "Binaries built in $(BUILD_DIR)/"
	@ls -la $(BUILD_DIR)/

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@$(GOCLEAN)
	@rm -f $(BINARY_NAME)
	@rm -rf $(BUILD_DIR)

# Run tests
test:
	$(GOTEST) -v ./...

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	@$(GOMOD) download
	@$(GOMOD) tidy
	@echo "Done!"

# Run linter (requires golangci-lint)
lint:
	golangci-lint run

# Install to GOPATH/bin
install: build
	@echo "Installing to $(GOPATH)/bin/$(BINARY_NAME)..."
	@mv $(BINARY_NAME) $(GOPATH)/bin/
	@echo "Done! You can now run 'nota' from anywhere"

# Development: build and run
run: build
	@./$(BINARY_NAME) --help

# Build and start web viewer
serve: build
	@echo "Starting web viewer..."
	@./$(BINARY_NAME) serve

# Build and extract sessions
extract: build
	@./$(BINARY_NAME) extract

# Build and extract all sessions
extract-all: build
	@./$(BINARY_NAME) extract --all

# Demo: show off all features
demo: build
	@echo ""
	@echo "=== nota demo ==="
	@echo ""
	@echo "1. Extracting AI sessions from this repository..."
	@echo ""
	@./$(BINARY_NAME) extract
	@echo ""
	@echo "2. Showing all sessions across all repositories..."
	@echo ""
	@./$(BINARY_NAME) extract --all | head -15
	@echo "   ... (truncated)"
	@echo ""
	@echo "3. Listing commits with linked AI transcripts..."
	@echo ""
	@./$(BINARY_NAME) notes list
	@echo ""
	@echo "4. To start the web viewer, run:"
	@echo "   make serve"
	@echo "   Then open http://localhost:8080"
	@echo ""
