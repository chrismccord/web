.PHONY: build test clean

# Default target - build for current platform
all:
	@echo "Building web for current platform..."
	@go build -o web .
	@echo "✅ Build complete: ./web"

# Build all platform binaries
build:
	@echo "Building web for all platforms..."
	@rm -f web web-darwin-arm64 web-darwin-amd64 web-linux-amd64
	@echo "Downloading Go dependencies..."
	@go mod download
	@echo "Building for macOS ARM64..."
	@GOOS=darwin GOARCH=arm64 go build -o web-darwin-arm64 .
	@echo "Building for macOS Intel..."
	@GOOS=darwin GOARCH=amd64 go build -o web-darwin-amd64 .
	@echo "Building for Linux x86_64..."
	@GOOS=linux GOARCH=amd64 go build -o web-linux-amd64 .
	@echo "Creating local symlink..."
	@ln -sf web-$(shell uname -s | tr '[:upper:]' '[:lower:]')-$(shell uname -m | sed 's/x86_64/amd64/') web
	@echo "✅ Build complete!"
	@echo "Binaries:"
	@echo "  web-darwin-arm64 ($(shell du -h web-darwin-arm64 2>/dev/null | cut -f1 || echo 'N/A')) - macOS Apple Silicon"
	@echo "  web-darwin-amd64 ($(shell du -h web-darwin-amd64 2>/dev/null | cut -f1 || echo 'N/A')) - macOS Intel"
	@echo "  web-linux-amd64  ($(shell du -h web-linux-amd64 2>/dev/null | cut -f1 || echo 'N/A')) - Linux x86_64"
	@echo "  web -> $(shell readlink web 2>/dev/null || echo 'local binary')"

# Run tests
test: build
	@echo "🧪 Running comprehensive test suite..."
	@go test -v -timeout=300s

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -f web web-darwin-arm64 web-darwin-amd64 web-linux-amd64
	@rm -f test-screenshot-*.png
	@rm -rf ~/.web-firefox/profiles/test-*
	@echo "✅ Clean complete"
