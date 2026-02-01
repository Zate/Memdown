.PHONY: test test-unit test-fuzz build install clean mcp-config

# Build
build:
	go build -o ctx .

# Build and install (binary, database, skill, hooks, CLAUDE.md)
install: build
	./ctx install --bin-dir ~/.local/bin

# All tests
test:
	go test -v ./...

# Unit tests only
test-unit:
	go test -v -short ./internal/...

# Fuzz testing
test-fuzz:
	go test -fuzz=FuzzQueryParser -fuzztime=30s ./internal/query/

# Coverage
test-coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Show MCP configuration for Claude Desktop
mcp-config: build
	./ctx install --mcp

# Clean
clean:
	rm -f ctx coverage.out coverage.html
