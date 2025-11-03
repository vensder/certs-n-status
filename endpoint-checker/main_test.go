package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// TestLoadEndpoints tests the endpoint loading functionality
func TestLoadEndpoints(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		wantCount  int
		wantErr    bool
		wantScheme string
	}{
		{
			name: "valid endpoints with https",
			content: `https://example.com
https://google.com
https://github.com`,
			wantCount:  3,
			wantErr:    false,
			wantScheme: "https://",
		},
		{
			name: "endpoints without scheme",
			content: `example.com
google.com`,
			wantCount:  2,
			wantErr:    false,
			wantScheme: "https://",
		},
		{
			name: "endpoints with comments and empty lines",
			content: `# This is a comment
https://example.com

# Another comment
https://google.com

`,
			wantCount: 2,
			wantErr:   false,
		},
		{
			name: "mixed http and https",
			content: `http://example.com
https://google.com`,
			wantCount: 2,
			wantErr:   false,
		},
		{
			name:      "empty file",
			content:   "",
			wantCount: 0,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary file
			tmpfile, err := os.CreateTemp("", "endpoints-*.lst")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(tmpfile.Name())

			// Write test content
			if _, err := tmpfile.Write([]byte(tt.content)); err != nil {
				t.Fatal(err)
			}
			if err := tmpfile.Close(); err != nil {
				t.Fatal(err)
			}

			// Create checker with test config
			config := Config{
				EndpointsFile: tmpfile.Name(),
				RedisAddr:     "localhost:6379",
			}
			checker := NewEndpointChecker(config)

			// Load endpoints
			endpoints, err := checker.loadEndpoints()

			// Check error expectation
			if (err != nil) != tt.wantErr {
				t.Errorf("loadEndpoints() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Check count
			if len(endpoints) != tt.wantCount {
				t.Errorf("loadEndpoints() got %d endpoints, want %d", len(endpoints), tt.wantCount)
			}

			// Check scheme if specified
			if tt.wantScheme != "" && len(endpoints) > 0 {
				for _, endpoint := range endpoints {
					if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
						t.Errorf("endpoint %s missing http/https scheme", endpoint)
					}
				}
			}
		})
	}
}

// TestCheckHTTPStatus tests HTTP status checking
func TestCheckHTTPStatus(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		wantStatusCode int
		wantErr        bool
	}{
		{"200 OK", http.StatusOK, 200, false},
		{"404 Not Found", http.StatusNotFound, 404, false},
		{"500 Internal Server Error", http.StatusInternalServerError, 500, false},
		{"301 Redirect", http.StatusMovedPermanently, 301, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			// Create checker
			config := Config{
				RedisAddr: "localhost:6379",
			}
			checker := NewEndpointChecker(config)

			// Check status
			statusCode, err := checker.checkHTTPStatus(server.URL)

			// Verify results
			if (err != nil) != tt.wantErr {
				t.Errorf("checkHTTPStatus() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if statusCode != tt.wantStatusCode {
				t.Errorf("checkHTTPStatus() = %v, want %v", statusCode, tt.wantStatusCode)
			}
		})
	}
}

// TestCheckHTTPStatusTimeout tests timeout handling
func TestCheckHTTPStatusTimeout(t *testing.T) {
	// Create server that delays response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(15 * time.Second) // Longer than client timeout
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := Config{
		RedisAddr: "localhost:6379",
	}
	checker := NewEndpointChecker(config)

	// This should timeout
	_, err := checker.checkHTTPStatus(server.URL)
	if err == nil {
		t.Error("checkHTTPStatus() expected timeout error, got nil")
	}
}

// TestStoreHTTPStatus tests Redis storage (requires Redis running)
func TestStoreHTTPStatus(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create Redis client for testing
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   15, // Use different DB for testing
	})
	ctx := context.Background()

	// Check if Redis is available
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skip("Redis not available, skipping integration test")
	}
	defer rdb.Close()

	// Clean up test data
	defer rdb.FlushDB(ctx)

	config := Config{
		RedisAddr: "localhost:6379",
		RedisDB:   15,
	}
	checker := NewEndpointChecker(config)

	testURL := "https://example.com"
	testStatus := 200

	// Store status
	err := checker.storeHTTPStatus(testURL, testStatus)
	if err != nil {
		t.Fatalf("storeHTTPStatus() error = %v", err)
	}

	// Verify stored status
	statusKey := fmt.Sprintf("status:%s", testURL)
	storedStatus, err := rdb.Get(ctx, statusKey).Int()
	if err != nil {
		t.Fatalf("Failed to get stored status: %v", err)
	}

	if storedStatus != testStatus {
		t.Errorf("Stored status = %d, want %d", storedStatus, testStatus)
	}

	// Verify timestamp was stored
	timestampKey := fmt.Sprintf("status_updated:%s", testURL)
	timestamp, err := rdb.Get(ctx, timestampKey).Int64()
	if err != nil {
		t.Fatalf("Failed to get stored timestamp: %v", err)
	}

	// Timestamp should be recent (within last minute)
	now := time.Now().Unix()
	if now-timestamp > 60 {
		t.Errorf("Timestamp too old: %d, now: %d", timestamp, now)
	}
}

// TestStoreSSLExpiration tests SSL expiration storage (requires Redis)
func TestStoreSSLExpiration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   15,
	})
	ctx := context.Background()

	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skip("Redis not available, skipping integration test")
	}
	defer rdb.Close()
	defer rdb.FlushDB(ctx)

	config := Config{
		RedisAddr: "localhost:6379",
		RedisDB:   15,
	}
	checker := NewEndpointChecker(config)

	testURL := "https://example.com"
	testExpiration := time.Now().Add(90 * 24 * time.Hour) // 90 days from now

	// Store SSL expiration
	err := checker.storeSSLExpiration(testURL, testExpiration)
	if err != nil {
		t.Fatalf("storeSSLExpiration() error = %v", err)
	}

	// Verify stored expiration
	sslKey := fmt.Sprintf("ssl:%s", testURL)
	storedTimestamp, err := rdb.Get(ctx, sslKey).Int64()
	if err != nil {
		t.Fatalf("Failed to get stored SSL expiration: %v", err)
	}

	if storedTimestamp != testExpiration.Unix() {
		t.Errorf("Stored expiration = %d, want %d", storedTimestamp, testExpiration.Unix())
	}
}

// TestCheckAllStatuses tests concurrent status checking
func TestCheckAllStatuses(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create multiple test servers
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server1.Close()

	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server2.Close()

	// Create Redis client for testing
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   15,
	})
	ctx := context.Background()

	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skip("Redis not available, skipping integration test")
	}
	defer rdb.Close()
	defer rdb.FlushDB(ctx)

	config := Config{
		RedisAddr: "localhost:6379",
		RedisDB:   15,
	}
	checker := NewEndpointChecker(config)

	endpoints := []string{server1.URL, server2.URL}

	// Check all statuses
	checker.checkAllStatuses(endpoints)

	// Verify results in Redis
	status1, err := rdb.Get(ctx, fmt.Sprintf("status:%s", server1.URL)).Int()
	if err != nil {
		t.Fatalf("Failed to get status for server1: %v", err)
	}
	if status1 != http.StatusOK {
		t.Errorf("Server1 status = %d, want %d", status1, http.StatusOK)
	}

	status2, err := rdb.Get(ctx, fmt.Sprintf("status:%s", server2.URL)).Int()
	if err != nil {
		t.Fatalf("Failed to get status for server2: %v", err)
	}
	if status2 != http.StatusNotFound {
		t.Errorf("Server2 status = %d, want %d", status2, http.StatusNotFound)
	}
}

// BenchmarkCheckHTTPStatus benchmarks HTTP status checking
func BenchmarkCheckHTTPStatus(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := Config{
		RedisAddr: "localhost:6379",
	}
	checker := NewEndpointChecker(config)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = checker.checkHTTPStatus(server.URL)
	}
}

// BenchmarkCheckAllStatuses benchmarks concurrent checking
func BenchmarkCheckAllStatuses(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping benchmark in short mode")
	}

	// Create test servers
	servers := make([]*httptest.Server, 10)
	endpoints := make([]string, 10)
	for i := 0; i < 10; i++ {
		servers[i] = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		endpoints[i] = servers[i].URL
		defer servers[i].Close()
	}

	config := Config{
		RedisAddr: "localhost:6379",
		RedisDB:   15,
	}
	checker := NewEndpointChecker(config)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		checker.checkAllStatuses(endpoints)
	}
}
