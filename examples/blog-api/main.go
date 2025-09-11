package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/priyanshu-shubham/gofastapi"
)

// Domain Models
type User struct {
	ID       string
	Username string
	Role     string
}

type Post struct {
	ID        string
	Title     string
	Content   string
	Author    string
	Tags      []string
	Draft     bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Auth Dependency - JWT Token Validation
type AuthDependency struct {
	validTokens map[string]User // In production, use proper JWT validation
}

type AuthRequest struct {
	Token string `header:"Authorization"`
}

type AuthResponse struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

func NewAuthDependency() *AuthDependency {
	return &AuthDependency{
		validTokens: map[string]User{
			"Bearer valid-token": {
				ID:       "user-123",
				Username: "john_doe",
				Role:     "admin",
			},
			"Bearer author-token": {
				ID:       "user-456",
				Username: "jane_smith",
				Role:     "author",
			},
		},
	}
}

func (d *AuthDependency) Handle(ctx context.Context, req AuthRequest) (AuthResponse, error) {
	if req.Token == "" {
		return AuthResponse{}, gofastapi.NewError(http.StatusUnauthorized, "Authorization header is required")
	}

	user, exists := d.validTokens[req.Token]
	if !exists {
		return AuthResponse{}, gofastapi.NewError(http.StatusUnauthorized, "Invalid authentication token")
	}

	return AuthResponse{
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.Role,
	}, nil
}

// Rate Limiting Dependency
type RateLimitDependency struct {
	requests map[string]int
	limit    int
}

type RateLimitRequest struct {
	UserID string `dep:"auth.UserID"`
}

type RateLimitResponse struct {
	Allowed   bool `json:"allowed"`
	Remaining int  `json:"remaining"`
}

func NewRateLimitDependency(limit int) *RateLimitDependency {
	return &RateLimitDependency{
		requests: make(map[string]int),
		limit:    limit,
	}
}

func (d *RateLimitDependency) Handle(ctx context.Context, req RateLimitRequest) (RateLimitResponse, error) {
	d.requests[req.UserID]++
	remaining := d.limit - d.requests[req.UserID]

	if remaining < 0 {
		return RateLimitResponse{}, gofastapi.NewError(http.StatusTooManyRequests, "Rate limit exceeded")
	}

	return RateLimitResponse{
		Allowed:   true,
		Remaining: remaining,
	}, nil
}

// Create Post Handler
type CreatePostRequest struct {
	Auth      AuthResponse      `dep:"auth"`
	RateLimit RateLimitResponse `dep:"rate_limit"`

	CategoryID string `path:"category_id" validate:"required,uuid" description:"Category ID" example:"550e8400-e29b-41d4-a716-446655440000"`

	Title   string   `json:"title" validate:"required,min=3,max=100" description:"Post title" example:"Getting Started with GoFastAPI"`
	Content string   `json:"content" validate:"required,min=10" description:"Post content" example:"This is a comprehensive guide..."`
	Tags    []string `json:"tags" validate:"max=5,dive,min=1,max=20" description:"Post tags" example:"[\"golang\", \"api\", \"tutorial\"]"`

	Draft bool `query:"draft" description:"Save as draft" default:"false"`
}

type CreatePostResponse struct {
	ID        string    `json:"id" description:"Post ID" example:"post-1234567890"`
	Title     string    `json:"title" description:"Post title"`
	Author    string    `json:"author" description:"Post author"`
	CreatedAt time.Time `json:"created_at" description:"Creation timestamp"`
	Draft     bool      `json:"draft" description:"Draft status"`
}

func CreatePostHandler(ctx context.Context, req CreatePostRequest) (CreatePostResponse, error) {
	// Check if user has permission to create posts
	if req.Auth.Role != "admin" && req.Auth.Role != "author" {
		return CreatePostResponse{}, gofastapi.NewError(http.StatusForbidden, "Insufficient permissions to create posts")
	}

	// Simulate post creation
	postID := fmt.Sprintf("post-%d", time.Now().Unix())

	log.Printf("Creating post '%s' in category %s by user %s", req.Title, req.CategoryID, req.Auth.Username)

	return CreatePostResponse{
		ID:        postID,
		Title:     req.Title,
		Author:    req.Auth.Username,
		CreatedAt: time.Now(),
		Draft:     req.Draft,
	}, nil
}

// Get Post Handler
type GetPostRequest struct {
	Auth      AuthResponse      `dep:"auth"`
	RateLimit RateLimitResponse `dep:"rate_limit"`

	PostID int `path:"post_id" validate:"required,min=1" description:"Post ID" example:"123"`
}

type GetPostResponse struct {
	ID        int       `json:"id" description:"Post ID"`
	Title     string    `json:"title" description:"Post title"`
	Content   string    `json:"content" description:"Post content"`
	Author    string    `json:"author" description:"Post author"`
	Tags      []string  `json:"tags" description:"Post tags"`
	CreatedAt time.Time `json:"created_at" description:"Creation timestamp"`
}

func GetPostHandler(ctx context.Context, req GetPostRequest) (GetPostResponse, error) {
	// Simulate fetching post from database
	return GetPostResponse{
		ID:        req.PostID,
		Title:     "Getting Started with GoFastAPI",
		Content:   "GoFastAPI is a powerful framework for building type-safe APIs in Go...",
		Author:    "john_doe",
		Tags:      []string{"golang", "api", "tutorial"},
		CreatedAt: time.Now().Add(-24 * time.Hour),
	}, nil
}

// List Posts Handler
type ListPostsRequest struct {
	Page     int    `query:"page" validate:"min=1" default:"1" description:"Page number"`
	PageSize int    `query:"page_size" validate:"min=1,max=100" default:"10" description:"Items per page"`
	Author   string `query:"author" description:"Filter by author"`
	Tag      string `query:"tag" description:"Filter by tag"`
}

type ListPostsResponse struct {
	Posts      []PostSummary `json:"posts"`
	Total      int           `json:"total"`
	Page       int           `json:"page"`
	PageSize   int           `json:"page_size"`
	TotalPages int           `json:"total_pages"`
}

type PostSummary struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Author    string    `json:"author"`
	CreatedAt time.Time `json:"created_at"`
	Tags      []string  `json:"tags"`
}

func ListPostsHandler(ctx context.Context, req ListPostsRequest) (ListPostsResponse, error) {
	// Simulate fetching posts from database
	posts := []PostSummary{
		{
			ID:        "post-1",
			Title:     "Introduction to GoFastAPI",
			Author:    "john_doe",
			CreatedAt: time.Now().Add(-48 * time.Hour),
			Tags:      []string{"golang", "tutorial"},
		},
		{
			ID:        "post-2",
			Title:     "Building REST APIs with Go",
			Author:    "jane_smith",
			CreatedAt: time.Now().Add(-24 * time.Hour),
			Tags:      []string{"golang", "api", "rest"},
		},
	}

	total := len(posts)
	totalPages := (total + req.PageSize - 1) / req.PageSize

	return ListPostsResponse{
		Posts:      posts,
		Total:      total,
		Page:       req.Page,
		PageSize:   req.PageSize,
		TotalPages: totalPages,
	}, nil
}

// Health Check Handler
type HealthResponse struct {
	Status    string            `json:"status" description:"Service status"`
	Timestamp time.Time         `json:"timestamp" description:"Current timestamp"`
	Version   string            `json:"version" description:"API version"`
	Services  map[string]string `json:"services" description:"Service dependencies status"`
}

func HealthHandler(ctx context.Context, req struct{}) (HealthResponse, error) {
	return HealthResponse{
		Status:    "healthy",
		Timestamp: time.Now(),
		Version:   "2.0.0",
		Services: map[string]string{
			"database": "connected",
			"cache":    "connected",
			"queue":    "connected",
		},
	}, nil
}

// Middleware
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("[%s] %s %s - %v",
			r.Method,
			r.URL.Path,
			r.RemoteAddr,
			time.Since(start))
	})
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func main() {
	// Create router with OpenAPI documentation
	r := gofastapi.NewWithOpenAPI(
		"Blog API",
		"2.0.0",
		"A comprehensive blog management API with authentication and rate limiting",
	)

	// Configure servers
	r.AddServer("http://localhost:8080", "Local development server")
	r.AddServer("https://api.blog.example.com", "Production server")

	// Register dependencies
	r.RegisterDependency("auth", NewAuthDependency(), gofastapi.SecuritySchemeBearer)
	r.RegisterDependency("rate_limit", NewRateLimitDependency(100))

	// Public endpoints
	r.GET("/health", HealthHandler)
	r.GET("/posts", ListPostsHandler)

	// Protected endpoints
	r.POST("/categories/{category_id}/posts", CreatePostHandler)
	r.GET("/posts/{post_id}", GetPostHandler)

	// API documentation
	r.ServeOpenAPIJSON("/openapi.json")
	// API docs UI
	r.ServeDocs("http://localhost:8080", "/docs", nil)

	// Apply middleware
	r.Use(corsMiddleware)
	r.Use(loggingMiddleware)

	// Start server
	fmt.Println("🚀 Blog API Server")
	fmt.Println("📍 Server:  http://localhost:8080")
	fmt.Println("📚 OpenAPI: http://localhost:8080/openapi.json")
	fmt.Println("📖 Docs:    http://localhost:8080/docs")
	fmt.Println("🔑 Auth:    Use 'Bearer valid-token' or 'Bearer author-token'")
	fmt.Println("")

	log.Fatal(http.ListenAndServe(":8080", r))
}
