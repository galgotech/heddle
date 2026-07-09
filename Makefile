.PHONY: all build clean test fmt lint

# Default target
all: build

# Build the heddle compiler/runtime binary into the bin/ directory
build:
	mkdir -p bin
	go build -o bin/heddle cmd/heddle/main.go

# Clean build artifacts
clean:
	rm -rf bin

# Run tests
test:
	go test ./...

# Format all Go source code
fmt:
	go fmt ./...

# Run linter
lint:
	golangci-lint run
