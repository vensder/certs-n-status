package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type Config struct {
	StatusCheckInterval time.Duration
	SSLCheckInterval    time.Duration
	EndpointsFile       string
	RedisAddr           string
	RedisPassword       string
	RedisDB             int
}

type EndpointChecker struct {
	config      Config
	redisClient *redis.Client
	ctx         context.Context
	httpClient  *http.Client
}

func NewEndpointChecker(config Config) *EndpointChecker {
	// Create Redis client
	rdb := redis.NewClient(&redis.Options{
		Addr:     config.RedisAddr,
		Password: config.RedisPassword,
		DB:       config.RedisDB,
	})

	// Create HTTP client with timeout
	httpClient := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: false,
			},
		},
	}

	return &EndpointChecker{
		config:      config,
		redisClient: rdb,
		ctx:         context.Background(),
		httpClient:  httpClient,
	}
}

func (ec *EndpointChecker) loadEndpoints() ([]string, error) {
	file, err := os.Open(ec.config.EndpointsFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open endpoints file: %w", err)
	}
	defer file.Close()

	var endpoints []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Ensure URL has scheme
		if !strings.HasPrefix(line, "http://") && !strings.HasPrefix(line, "https://") {
			line = "https://" + line
		}
		endpoints = append(endpoints, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading endpoints file: %w", err)
	}

	return endpoints, nil
}

func (ec *EndpointChecker) checkHTTPStatus(url string) (int, error) {
	resp, err := ec.httpClient.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

func (ec *EndpointChecker) checkSSLExpiration(url string) (time.Time, error) {
	// Only check HTTPS URLs
	if !strings.HasPrefix(url, "https://") {
		return time.Time{}, fmt.Errorf("not an HTTPS URL")
	}

	// Extract hostname
	hostname := strings.TrimPrefix(url, "https://")
	hostname = strings.Split(hostname, "/")[0]
	hostname = strings.Split(hostname, ":")[0]

	conn, err := tls.Dial("tcp", hostname+":443", &tls.Config{
		InsecureSkipVerify: false,
	})
	if err != nil {
		return time.Time{}, err
	}
	defer conn.Close()

	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return time.Time{}, fmt.Errorf("no certificates found")
	}

	// Return the expiration of the first certificate (leaf certificate)
	return certs[0].NotAfter, nil
}

func (ec *EndpointChecker) storeHTTPStatus(url string, statusCode int) error {
	pipe := ec.redisClient.Pipeline()

	// Store status code
	statusKey := fmt.Sprintf("status:%s", url)
	pipe.Set(ec.ctx, statusKey, statusCode, 0)

	// Store last update timestamp
	timestampKey := fmt.Sprintf("status_updated:%s", url)
	pipe.Set(ec.ctx, timestampKey, time.Now().Unix(), 0)

	_, err := pipe.Exec(ec.ctx)
	return err
}

func (ec *EndpointChecker) storeSSLExpiration(url string, expiration time.Time) error {
	pipe := ec.redisClient.Pipeline()

	// Store SSL expiration as Unix timestamp
	sslKey := fmt.Sprintf("ssl:%s", url)
	pipe.Set(ec.ctx, sslKey, expiration.Unix(), 0)

	// Store last check timestamp
	timestampKey := fmt.Sprintf("ssl_updated:%s", url)
	pipe.Set(ec.ctx, timestampKey, time.Now().Unix(), 0)

	_, err := pipe.Exec(ec.ctx)
	return err
}

func (ec *EndpointChecker) checkEndpointStatus(url string) {
	statusCode, err := ec.checkHTTPStatus(url)
	if err != nil {
		log.Printf("[ERROR] Failed to check status for %s: %v", url, err)
		// Store error code as 0
		statusCode = 0
	}

	if err := ec.storeHTTPStatus(url, statusCode); err != nil {
		log.Printf("[ERROR] Failed to store status for %s: %v", url, err)
	} else {
		log.Printf("[INFO] Status check: %s -> %d", url, statusCode)
	}
}

func (ec *EndpointChecker) checkEndpointSSL(url string) {
	expiration, err := ec.checkSSLExpiration(url)
	if err != nil {
		log.Printf("[ERROR] Failed to check SSL for %s: %v", url, err)
		return
	}

	if err := ec.storeSSLExpiration(url, expiration); err != nil {
		log.Printf("[ERROR] Failed to store SSL expiration for %s: %v", url, err)
	} else {
		daysLeft := int(time.Until(expiration).Hours() / 24)
		log.Printf("[INFO] SSL check: %s -> expires in %d days (%s)", url, daysLeft, expiration.Format("2006-01-02"))
	}
}

func (ec *EndpointChecker) runStatusChecker(endpoints []string) {
	ticker := time.NewTicker(ec.config.StatusCheckInterval)
	defer ticker.Stop()

	// Initial check
	ec.checkAllStatuses(endpoints)

	for range ticker.C {
		ec.checkAllStatuses(endpoints)
	}
}

func (ec *EndpointChecker) runSSLChecker(endpoints []string) {
	ticker := time.NewTicker(ec.config.SSLCheckInterval)
	defer ticker.Stop()

	// Initial check
	ec.checkAllSSL(endpoints)

	for range ticker.C {
		ec.checkAllSSL(endpoints)
	}
}

func (ec *EndpointChecker) checkAllStatuses(endpoints []string) {
	var wg sync.WaitGroup
	for _, url := range endpoints {
		wg.Add(1)
		go func(u string) {
			defer wg.Done()
			ec.checkEndpointStatus(u)
		}(url)
	}
	wg.Wait()
}

func (ec *EndpointChecker) checkAllSSL(endpoints []string) {
	var wg sync.WaitGroup
	for _, url := range endpoints {
		// Only check HTTPS URLs
		if strings.HasPrefix(url, "https://") {
			wg.Add(1)
			go func(u string) {
				defer wg.Done()
				ec.checkEndpointSSL(u)
			}(url)
		}
	}
	wg.Wait()
}

func (ec *EndpointChecker) Start() error {
	// Test Redis connection
	if err := ec.redisClient.Ping(ec.ctx).Err(); err != nil {
		return fmt.Errorf("failed to connect to Redis: %w", err)
	}
	log.Println("[INFO] Connected to Redis successfully")

	// Load endpoints
	endpoints, err := ec.loadEndpoints()
	if err != nil {
		return err
	}
	log.Printf("[INFO] Loaded %d endpoints", len(endpoints))

	// Start checkers in separate goroutines
	go ec.runStatusChecker(endpoints)
	go ec.runSSLChecker(endpoints)

	// Keep the program running
	select {}
}

func main() {
	config := Config{
		StatusCheckInterval: 1 * time.Minute,
		SSLCheckInterval:    1 * time.Hour,
		EndpointsFile:       "endpoints.lst",
		RedisAddr:           "localhost:6379",
		RedisPassword:       "", // Set if needed
		RedisDB:             0,
	}

	// Allow configuration via environment variables
	if envInterval := os.Getenv("STATUS_CHECK_INTERVAL"); envInterval != "" {
		if d, err := time.ParseDuration(envInterval); err == nil {
			config.StatusCheckInterval = d
		}
	}
	if envInterval := os.Getenv("SSL_CHECK_INTERVAL"); envInterval != "" {
		if d, err := time.ParseDuration(envInterval); err == nil {
			config.SSLCheckInterval = d
		}
	}
	if envFile := os.Getenv("ENDPOINTS_FILE"); envFile != "" {
		config.EndpointsFile = envFile
	}
	if envAddr := os.Getenv("REDIS_ADDR"); envAddr != "" {
		config.RedisAddr = envAddr
	}
	if envPass := os.Getenv("REDIS_PASSWORD"); envPass != "" {
		config.RedisPassword = envPass
	}

	log.Printf("[INFO] Starting endpoint checker...")
	log.Printf("[INFO] Status check interval: %s", config.StatusCheckInterval)
	log.Printf("[INFO] SSL check interval: %s", config.SSLCheckInterval)

	checker := NewEndpointChecker(config)
	if err := checker.Start(); err != nil {
		log.Fatalf("[FATAL] %v", err)
	}
}
