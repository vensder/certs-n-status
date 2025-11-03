.PHONY: test test-short test-integration test-coverage test-verbose benchmark clean run

# Run all tests
test:
	go test -v ./...

# Run only unit tests (skip integration tests)
test-short:
	go test -short -v ./...

# Run only integration tests (requires Redis)
test-integration:
	go test -run Integration -v ./...

# Run tests with coverage report
test-coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Run tests with race detector
test-race:
	go test -race -v ./...

# Run tests with verbose output
test-verbose:
	go test -v -count=1 ./...

# Run benchmarks
benchmark:
	go test -bench=. -benchmem ./...

# Run specific test
test-one:
	@read -p "Enter test name: " test; \
	go test -run $$test -v ./...

# Clean test cache and coverage files
clean:
	go clean -testcache
	rm -f coverage.out coverage.html

# Run the application
run:
	go run main.go

# Build the application
build:
	go build -o endpoint-checker main.go

# Install dependencies
deps:
	go mod download
	go mod tidy

# Format code
fmt:
	go fmt ./...

# Run linter (requires golangci-lint)
lint:
	golangci-lint run

# Run all checks (format, lint, test)
check: fmt lint test

# Display help
help:
	@echo "Available targets:"
	@echo "  test            - Run all tests"
	@echo "  test-short      - Run only unit tests (no Redis required)"
	@echo "  test-integration - Run only integration tests (requires Redis)"
	@echo "  test-coverage   - Run tests with coverage report"
	@echo "  test-race       - Run tests with race detector"
	@echo "  test-verbose    - Run tests with verbose output"
	@echo "  benchmark       - Run benchmarks"
	@echo "  test-one        - Run specific test by name"
	@echo "  clean           - Clean test cache and coverage files"
	@echo "  run             - Run the application"
	@echo "  build           - Build the application"
	@echo "  deps            - Install/update dependencies"
	@echo "  fmt             - Format code"
	@echo "  lint            - Run linter (requires golangci-lint)"
	@echo "  check           - Run format, lint, and tests"
	@echo "  help            - Display this help message"
