.PHONY: test test-unit test-fuzz build clean

CGO_CFLAGS := -DSQLITE_ENABLE_FTS5
CGO_LDFLAGS := -lm

# Build
build:
	CGO_CFLAGS="$(CGO_CFLAGS)" CGO_LDFLAGS="$(CGO_LDFLAGS)" go build -o ctx .

# All tests
test:
	CGO_CFLAGS="$(CGO_CFLAGS)" CGO_LDFLAGS="$(CGO_LDFLAGS)" go test -v ./...

# Unit tests only
test-unit:
	CGO_CFLAGS="$(CGO_CFLAGS)" CGO_LDFLAGS="$(CGO_LDFLAGS)" go test -v -short ./internal/...

# Fuzz testing
test-fuzz:
	CGO_CFLAGS="$(CGO_CFLAGS)" CGO_LDFLAGS="$(CGO_LDFLAGS)" go test -fuzz=FuzzQueryParser -fuzztime=30s ./internal/query/

# Coverage
test-coverage:
	CGO_CFLAGS="$(CGO_CFLAGS)" CGO_LDFLAGS="$(CGO_LDFLAGS)" go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Clean
clean:
	rm -f ctx coverage.out coverage.html
