package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type Config struct {
	RedisAddr     string
	RedisPassword string
	RedisDB       int
	ServerPort    string
}

type EndpointData struct {
	Endpoint         string
	StatusCode       int
	StatusText       string
	StatusClass      string
	SSLExpiration    *time.Time
	DaysLeft         *int
	SSLText          string
	SSLClass         string
	LastStatusUpdate *time.Time
	LastSSLUpdate    *time.Time
	UpdateText       string
	IsHTTPS          bool
}

type DashboardData struct {
	Endpoints       []EndpointData
	TotalEndpoints  int
	HealthyCount    int
	SSLWarningCount int
	CurrentTime     string
}

type Server struct {
	config      Config
	redisClient *redis.Client
	ctx         context.Context
	templates   *template.Template
}

func NewServer(config Config) (*Server, error) {
	// Create Redis client
	rdb := redis.NewClient(&redis.Options{
		Addr:     config.RedisAddr,
		Password: config.RedisPassword,
		DB:       config.RedisDB,
	})

	// Test Redis connection
	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	// Create template with custom functions
	funcMap := template.FuncMap{
		"add": func(a, b int) int { return a + b },
	}

	// Parse templates with custom functions
	tmpl, err := template.New("index.html").Funcs(funcMap).ParseFiles("templates/index.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse templates: %w", err)
	}

	return &Server{
		config:      config,
		redisClient: rdb,
		ctx:         ctx,
		templates:   tmpl,
	}, nil
}

func (s *Server) getAllEndpoints() ([]string, error) {
	endpoints := make(map[string]bool)

	// Get all status keys
	iter := s.redisClient.Scan(s.ctx, 0, "status:*", 0).Iterator()
	for iter.Next(s.ctx) {
		key := iter.Val()
		endpoint := strings.TrimPrefix(key, "status:")
		endpoints[endpoint] = true
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}

	// Get all ssl keys
	iter = s.redisClient.Scan(s.ctx, 0, "ssl:*", 0).Iterator()
	for iter.Next(s.ctx) {
		key := iter.Val()
		endpoint := strings.TrimPrefix(key, "ssl:")
		endpoints[endpoint] = true
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}

	// Convert map keys to slice
	result := make([]string, 0, len(endpoints))
	for endpoint := range endpoints {
		result = append(result, endpoint)
	}

	return result, nil
}

func (s *Server) getEndpointData(endpoint string) EndpointData {
	data := EndpointData{
		Endpoint: endpoint,
		IsHTTPS:  strings.HasPrefix(endpoint, "https://"),
	}

	// Get HTTP status
	statusKey := fmt.Sprintf("status:%s", endpoint)
	if statusStr, err := s.redisClient.Get(s.ctx, statusKey).Result(); err == nil {
		if code, err := strconv.Atoi(statusStr); err == nil {
			data.StatusCode = code
			data.StatusText = statusStr
		}
	}

	// Get status update time
	statusUpdatedKey := fmt.Sprintf("status_updated:%s", endpoint)
	if timestampStr, err := s.redisClient.Get(s.ctx, statusUpdatedKey).Result(); err == nil {
		if timestamp, err := strconv.ParseInt(timestampStr, 10, 64); err == nil {
			t := time.Unix(timestamp, 0).UTC()
			data.LastStatusUpdate = &t
		}
	}

	// Get SSL expiration (only for HTTPS)
	if data.IsHTTPS {
		sslKey := fmt.Sprintf("ssl:%s", endpoint)
		if sslStr, err := s.redisClient.Get(s.ctx, sslKey).Result(); err == nil {
			if timestamp, err := strconv.ParseInt(sslStr, 10, 64); err == nil {
				expDate := time.Unix(timestamp, 0).UTC()
				data.SSLExpiration = &expDate

				// Calculate days left
				now := time.Now().UTC()
				delta := expDate.Sub(now)
				days := int(delta.Hours() / 24)
				data.DaysLeft = &days
			}
		}

		// Get SSL update time
		sslUpdatedKey := fmt.Sprintf("ssl_updated:%s", endpoint)
		if timestampStr, err := s.redisClient.Get(s.ctx, sslUpdatedKey).Result(); err == nil {
			if timestamp, err := strconv.ParseInt(timestampStr, 10, 64); err == nil {
				t := time.Unix(timestamp, 0).UTC()
				data.LastSSLUpdate = &t
			}
		}
	}

	// Set display values
	data.StatusClass = getStatusClass(data.StatusCode)
	data.SSLClass = getSSLClass(data.DaysLeft)
	data.SSLText = getSSLText(data.IsHTTPS, data.DaysLeft)

	// Get last update
	var lastUpdate *time.Time
	if data.LastStatusUpdate != nil {
		lastUpdate = data.LastStatusUpdate
	}
	if data.LastSSLUpdate != nil && (lastUpdate == nil || data.LastSSLUpdate.After(*lastUpdate)) {
		lastUpdate = data.LastSSLUpdate
	}
	data.UpdateText = formatTimeAgo(lastUpdate)

	return data
}

func getStatusClass(statusCode int) string {
	if statusCode == 0 {
		return "status-error"
	} else if statusCode >= 200 && statusCode < 300 {
		return "status-success"
	} else if statusCode >= 300 && statusCode < 400 {
		return "status-redirect"
	} else if statusCode >= 400 && statusCode < 500 {
		return "status-client-error"
	} else if statusCode >= 500 && statusCode < 600 {
		return "status-server-error"
	}
	return "status-unknown"
}

func getSSLClass(daysLeft *int) string {
	if daysLeft == nil {
		return ""
	}
	days := *daysLeft
	if days < 0 {
		return "ssl-expired"
	} else if days < 7 {
		return "ssl-critical"
	} else if days < 30 {
		return "ssl-warning"
	}
	return "ssl-ok"
}

func getSSLText(isHTTPS bool, daysLeft *int) string {
	if !isHTTPS {
		return "HTTP only"
	}
	if daysLeft == nil {
		return "Checking..."
	}
	days := *daysLeft
	if days < 0 {
		return fmt.Sprintf("Expired %d days ago", -days)
	}
	return fmt.Sprintf("%d days left", days)
}

func formatTimeAgo(t *time.Time) string {
	if t == nil {
		return "Never"
	}

	now := time.Now().UTC()
	delta := now.Sub(*t)
	seconds := delta.Seconds()

	if seconds < 60 {
		return fmt.Sprintf("%ds ago", int(seconds))
	} else if seconds < 3600 {
		return fmt.Sprintf("%dm ago", int(seconds/60))
	} else if seconds < 86400 {
		return fmt.Sprintf("%dh ago", int(seconds/3600))
	}
	return fmt.Sprintf("%dd ago", int(seconds/86400))
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	endpoints, err := s.getAllEndpoints()
	if err != nil {
		http.Error(w, "Failed to get endpoints", http.StatusInternalServerError)
		log.Printf("[ERROR] Failed to get endpoints: %v", err)
		return
	}

	// Get data for all endpoints
	endpointData := make([]EndpointData, 0, len(endpoints))
	for _, endpoint := range endpoints {
		data := s.getEndpointData(endpoint)
		endpointData = append(endpointData, data)
	}

	// Sort by SSL expiration days (nil values go to the end)
	sort.Slice(endpointData, func(i, j int) bool {
		if endpointData[i].DaysLeft == nil && endpointData[j].DaysLeft == nil {
			return false
		}
		if endpointData[i].DaysLeft == nil {
			return false
		}
		if endpointData[j].DaysLeft == nil {
			return true
		}
		return *endpointData[i].DaysLeft < *endpointData[j].DaysLeft
	})

	// Calculate statistics
	healthyCount := 0
	sslWarningCount := 0
	for _, ep := range endpointData {
		if ep.StatusCode >= 200 && ep.StatusCode < 300 {
			healthyCount++
		}
		if ep.DaysLeft != nil && *ep.DaysLeft < 30 {
			sslWarningCount++
		}
	}

	dashboardData := DashboardData{
		Endpoints:       endpointData,
		TotalEndpoints:  len(endpointData),
		HealthyCount:    healthyCount,
		SSLWarningCount: sslWarningCount,
		CurrentTime:     time.Now().UTC().Format("15:04:05 MST"),
	}

	if err := s.templates.Execute(w, dashboardData); err != nil {
		http.Error(w, "Failed to render template", http.StatusInternalServerError)
		log.Printf("[ERROR] Failed to render template: %v", err)
	}
}

func (s *Server) handleAPIEndpoints(w http.ResponseWriter, r *http.Request) {
	endpoints, err := s.getAllEndpoints()
	if err != nil {
		http.Error(w, "Failed to get endpoints", http.StatusInternalServerError)
		return
	}

	endpointData := make([]EndpointData, 0, len(endpoints))
	for _, endpoint := range endpoints {
		data := s.getEndpointData(endpoint)
		endpointData = append(endpointData, data)
	}

	// Sort by SSL expiration days
	sort.Slice(endpointData, func(i, j int) bool {
		if endpointData[i].DaysLeft == nil && endpointData[j].DaysLeft == nil {
			return false
		}
		if endpointData[i].DaysLeft == nil {
			return false
		}
		if endpointData[j].DaysLeft == nil {
			return true
		}
		return *endpointData[i].DaysLeft < *endpointData[j].DaysLeft
	})

	response := map[string]interface{}{
		"endpoints": endpointData,
		"total":     len(endpointData),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *Server) Start() error {
	http.HandleFunc("/", s.handleIndex)
	http.HandleFunc("/api/endpoints", s.handleAPIEndpoints)

	log.Printf("[INFO] Starting Go dashboard server on port %s", s.config.ServerPort)
	log.Printf("[INFO] Access the dashboard at: http://localhost:%s", s.config.ServerPort)

	return http.ListenAndServe(":"+s.config.ServerPort, nil)
}

func main() {
	config := Config{
		RedisAddr:     getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),
		RedisDB:       getEnvInt("REDIS_DB", 0),
		ServerPort:    getEnv("SERVER_PORT", "8080"),
	}

	server, err := NewServer(config)
	if err != nil {
		log.Fatalf("[FATAL] Failed to create server: %v", err)
	}

	if err := server.Start(); err != nil {
		log.Fatalf("[FATAL] Server error: %v", err)
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}
