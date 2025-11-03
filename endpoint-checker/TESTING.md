# Testing Guide for Endpoint Checker

This guide explains how to test the endpoint checker application using Go's built-in testing framework.

## Test Organization

The tests are organized in `main_test.go` and include:

1. **Unit Tests** - Test individual functions without external dependencies
2. **Integration Tests** - Test Redis integration (require Redis running)
3. **Benchmarks** - Performance testing

## Running Tests

### Quick Start

```bash
# Run all tests
go test -v

# Run only unit tests (no Redis required)
go test -short -v

# Using Makefile
make test
make test-short
```

### Test Categories

#### 1. Unit Tests (No Redis Required)

```bash
# Run unit tests only
go test -short -v

# Or with make
make test-short
```

Unit tests include:
- `TestLoadEndpoints` - Endpoint file parsing
- `TestCheckHTTPStatus` - HTTP status checking with mock servers
- `TestCheckHTTPStatusTimeout` - Timeout handling

#### 2. Integration Tests (Requires Redis)

```bash
# Run integration tests
go test -v

# Or with make
make test-integration
```

Integration tests include:
- `TestStoreHTTPStatus` - Redis storage for HTTP status
- `TestStoreSSLExpiration` - Redis storage for SSL expiration
- `TestCheckAllStatuses` - Concurrent endpoint checking

**Note:** Integration tests use Redis DB 15 to avoid conflicts with production data.

### Coverage Reports

```bash
# Generate coverage report
go test -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html

# Or with make
make test-coverage
```

Open `coverage.html` in your browser to see detailed coverage.

### Race Detection

```bash
# Run tests with race detector (detects concurrency issues)
go test -race -v

# Or with make
make test-race
```

### Benchmarks

```bash
# Run all benchmarks
go test -bench=. -benchmem

# Run specific benchmark
go test -bench=BenchmarkCheckHTTPStatus -benchmem

# Or with make
make benchmark
```

Available benchmarks:
- `BenchmarkCheckHTTPStatus` - Single HTTP request performance
- `BenchmarkCheckAllStatuses` - Concurrent checking performance

## Test Structure

### Example Test Function

```go
func TestLoadEndpoints(t *testing.T) {
    // Arrange - setup test data
    content := "https://example.com\nhttps://google.com"
    tmpfile := createTempFile(content)
    defer os.Remove(tmpfile)
    
    // Act - execute the function
    endpoints, err := checker.loadEndpoints()
    
    // Assert - verify results
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if len(endpoints) != 2 {
        t.Errorf("got %d endpoints, want 2", len(endpoints))
    }
}
```

### Table-Driven Tests

```go
func TestCheckHTTPStatus(t *testing.T) {
    tests := []struct {
        name       string
        statusCode int
        wantErr    bool
    }{
        {"200 OK", 200, false},
        {"404 Not Found", 404, false},
        {"500 Error", 500, false},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation
        })
    }
}
```

## CI/CD Integration

### GitHub Actions Example

```yaml
name: Tests
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    services:
      redis:
        image: redis:alpine
        ports:
          - 6379:6379
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      - run: go test -v -race -coverprofile=coverage.out
      - run: go tool cover -func=coverage.out
```

### GitLab CI Example

```yaml
test:
  image: golang:1.21
  services:
    - redis:alpine
  script:
    - go test -v -race -coverprofile=coverage.out
    - go tool cover -func=coverage.out
```

## Makefile Commands

We provide a Makefile for convenience:

| Command | Description |
|---------|-------------|
| `make test` | Run all tests |
| `make test-short` | Run unit tests only (no Redis) |
| `make test-integration` | Run integration tests only |
| `make test-coverage` | Generate coverage report |
| `make test-race` | Run with race detector |
| `make benchmark` | Run benchmarks |
| `make test-one` | Run specific test by name |
| `make clean` | Clean test cache |
| `make help` | Show all available commands |

## Best Practices

### 1. Test Naming

- Test functions: `TestFunctionName`
- Benchmark functions: `BenchmarkFunctionName`
- Use descriptive names: `TestLoadEndpoints_WithComments`

### 2. Skip Tests When Dependencies Unavailable

```go
func TestWithRedis(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test in short mode")
    }
    
    if err := redis.Ping(); err != nil {
        t.Skip("Redis not available")
    }
    
    // Test implementation
}
```

### 3. Clean Up Resources

```go
func TestExample(t *testing.T) {
    tmpfile := createTempFile()
    defer os.Remove(tmpfile)  // Always clean up
    
    rdb := connectRedis()
    defer rdb.Close()
    defer rdb.FlushDB()  // Clean test data
}
```

### 4. Use Test Helpers

```go
func createTestChecker(t *testing.T) *EndpointChecker {
    t.Helper()  // Mark as helper function
    config := Config{
        RedisAddr: "localhost:6379",
        RedisDB:   15,
    }
    return NewEndpointChecker(config)
}
```

### 5. Table-Driven Tests

Use table-driven tests for testing multiple scenarios:

```go
tests := []struct {
    name    string
    input   string
    want    int
    wantErr bool
}{
    {"valid", "200", 200, false},
    {"invalid", "abc", 0, true},
}
```

## Troubleshooting

### Redis Connection Issues

If integration tests fail with Redis connection errors:

```bash
# Check if Redis is running
redis-cli ping

# Start Redis with Docker
docker run -d -p 6379:6379 redis:alpine

# Or install locally
sudo apt-get install redis-server  # Ubuntu/Debian
brew install redis                  # macOS
```

### Test Cache Issues

If tests show cached results:

```bash
# Clear test cache
go clean -testcache

# Or with make
make clean
```

### Timeout Issues

If tests timeout, increase the timeout:

```bash
go test -timeout 30s -v
```

## Writing New Tests

### 1. Unit Test Template

```go
func TestNewFunction(t *testing.T) {
    // Arrange
    input := "test"
    
    // Act
    result := NewFunction(input)
    
    // Assert
    if result != expected {
        t.Errorf("got %v, want %v", result, expected)
    }
}
```

### 2. Integration Test Template

```go
func TestNewIntegration(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test")
    }
    
    // Setup
    rdb := connectTestRedis(t)
    defer rdb.Close()
    defer rdb.FlushDB(context.Background())
    
    // Test implementation
}
```

### 3. Benchmark Template

```go
func BenchmarkNewFunction(b *testing.B) {
    // Setup
    input := "test"
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        NewFunction(input)
    }
}
```

## Coverage Goals

- **Unit tests**: Aim for >80% coverage
- **Critical paths**: Aim for 100% coverage (status checking, Redis storage)
- **Error handling**: Ensure all error paths are tested

## Resources

- [Go Testing Package](https://pkg.go.dev/testing)
- [Go Testing Best Practices](https://go.dev/doc/tutorial/add-a-test)
- [Table Driven Tests](https://dave.cheney.net/2019/05/07/prefer-table-driven-tests)
- [Go Test Comments](https://golang.org/cmd/go/#hdr-Testing_flags)

## 🎯 Go Testing Best Practices Used:

Table-driven tests - Test multiple scenarios efficiently
testing.Short() - Skip integration tests when -short flag used
Test helpers - httptest.NewServer for HTTP mocking
Cleanup with defer - Proper resource cleanup
Separate test DB - Use Redis DB 15 for tests
Benchmarks - Performance testing with -bench
Coverage - Track test coverage
Race detection - Find concurrency bugs

This follows Go's idiomatic testing approach - everything uses the standard testing package with no external testing frameworks needed! 🎉
