package main

import (
	"context"
	"fmt"
	"iter"
	"log"
	"math/rand"
	"net/http"
	"time"

	"github.com/priyanshu-shubham/gofastapi"
)

type SystemAlert struct {
	Level     string                 `json:"level" description:"Alert level (info, warning, error)"`
	Service   string                 `json:"service" description:"Service name"`
	Message   string                 `json:"message" description:"Alert message"`
	Details   map[string]interface{} `json:"details,omitempty" description:"Additional details"`
	Timestamp time.Time              `json:"timestamp" description:"Alert timestamp"`
}

type AlertStreamRequest struct {
	Services []string `json:"services" description:"List of services to monitor"`
	MinLevel string   `json:"min_level" default:"info" description:"Minimum alert level (info, warning, error)"`
}

func streamSystemAlerts(ctx context.Context, req AlertStreamRequest) (iter.Seq[gofastapi.EventData[SystemAlert]], error) {
	return func(yield func(gofastapi.EventData[SystemAlert]) bool) {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()

		services := req.Services
		if len(services) == 0 {
			services = []string{"api-gateway", "database", "cache", "auth-service"}
		}

		levels := []string{"info", "warning", "error"}
		messages := map[string][]string{
			"info":    {"Service healthy", "Routine maintenance completed", "Performance metrics updated"},
			"warning": {"High memory usage detected", "Response time increased", "Disk space running low"},
			"error":   {"Service unavailable", "Database connection failed", "Critical error occurred"},
		}

		counter := 0

		for {
			select {
			case <-ticker.C:
				counter++

				// Generate a random alert
				service := services[rand.Intn(len(services))]
				level := levels[rand.Intn(len(levels))]
				levelMessages := messages[level]
				message := levelMessages[rand.Intn(len(levelMessages))]

				// Check minimum level filtering
				if !shouldIncludeLevel(level, req.MinLevel) {
					continue
				}

				event := gofastapi.EventData[SystemAlert]{
					Event: fmt.Sprintf("alert-%s", level),
					ID:    fmt.Sprintf("alert-%d", counter),
					Retry: 5000, // Retry after 5 seconds if connection lost
					Data: SystemAlert{
						Level:   level,
						Service: service,
						Message: message,
						Details: map[string]interface{}{
							"alert_id": counter,
							"node":     fmt.Sprintf("node-%d", rand.Intn(5)+1),
						},
						Timestamp: time.Now(),
					},
				}

				if !yield(event) {
					return
				}

			case <-ctx.Done():
				return
			}
		}
	}, nil
}

// Helper function
func shouldIncludeLevel(alertLevel, minLevel string) bool {
	levelOrder := map[string]int{
		"info":    1,
		"warning": 2,
		"error":   3,
	}
	return levelOrder[alertLevel] >= levelOrder[minLevel]
}

func main() {
	// Create router with OpenAPI documentation
	router := gofastapi.NewWithOpenAPI(
		"Stock Streaming API",
		"1.0.0",
		"Real-time stock prices and chat with Server-Sent Events",
	)

	// Add server info for OpenAPI
	router.AddServer("http://localhost:8080", "Development server")

	// Register SSE endpoints
	log.Println("Registering SSE routes...")

	// POST SSE endpoint for system alerts
	if err := router.SSEPOST("/alerts/stream", streamSystemAlerts); err != nil {
		log.Fatalf("Failed to register alert stream route: %v", err)
	}

	// API documentation endpoints
	router.ServeOpenAPIJSON("/openapi.json")
	router.ServeDocs("http://localhost:8080", "/docs", nil)

	log.Println("Server starting on :8080")
	log.Println("Visit http://localhost:8080 for endpoint information")
	log.Println("Visit http://localhost:8080/docs for API documentation")

	if err := http.ListenAndServe(":8080", router); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
