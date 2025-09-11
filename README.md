# GoFastAPI

[![Go Reference](https://pkg.go.dev/badge/github.com/priyanshu-shubham/gofastapi.svg)](https://pkg.go.dev/github.com/priyanshu-shubham/gofastapi)
[![Go Report Card](https://goreportcard.com/badge/github.com/priyanshu-shubham/gofastapi)](https://goreportcard.com/report/github.com/priyanshu-shubham/gofastapi)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A blazing-fast, type-safe Go web framework inspired by FastAPI, built on top of Gorilla Mux. Write APIs with automatic serialization, validation, dependency injection, and OpenAPI documentation generation - all with zero reflection at runtime.

## ✨ Features

- **🚀 Zero Runtime Reflection** - All reflection happens at startup, ensuring maximum performance
- **📝 Automatic OpenAPI Generation** - Get Swagger/OpenAPI 3.0 docs automatically from your code
- **🔒 Type-Safe Handlers** - Use structs for requests/responses with compile-time type checking
- **💉 Dependency Injection** - Built-in DI system with automatic resolution and caching
- **✅ Validation** - Automatic request validation using struct tags
- **🎯 Multiple Parameter Sources** - Extract data from path, query, headers, and body seamlessly
- **⚡ High Performance** - Optimized for speed with minimal overhead
- **🔐 Security Schemes** - Built-in support for Bearer, API Key, and Basic auth
- **🛠️ Extensible** - Easy to extend with custom validators and middleware

## 📦 Installation
```bash
go get github.com/priyanshu-shubham/gofastapi
```

## 🚀 Quick Start
```golang
package main

import (
    "context"
    "log"
    "net/http"

    "github.com/priyanshu-shubham/gofastapi"
)

// Define your request/response types
type CreateUserRequest struct {
    Name  string `json:"name" validate:"required,min=3"`
    Email string `json:"email" validate:"required,email"`
    Age   int    `json:"age" validate:"min=18,max=120"`
}

type CreateUserResponse struct {
    ID      string `json:"id"`
    Message string `json:"message"`
}

// Write your handler - just a regular function!
func CreateUser(ctx context.Context, req CreateUserRequest) (CreateUserResponse, error) {
    // Your business logic here
    return CreateUserResponse{
        ID:      "user-123",
        Message: "User " + req.Name + " created successfully",
    }, nil
}

func main() {
    // Create router with OpenAPI documentation
    r := gofastapi.NewWithOpenAPI(
        "My API",
        "1.0.0",
        "A simple user management API",
    )

    // Register your routes
    r.POST("/users", CreateUser)

    // Serve OpenAPI spec and docs
    r.ServeOpenAPIJSON("/openapi.json")
    r.ServeDocs("http://localhost:8080", "/docs", nil)

    log.Println("Server starting on :8080")
    log.Println("OpenAPI spec at http://localhost:8080/openapi.json")
    log.Println("Docs UI at http://localhost:8080/docs")
    log.Fatal(http.ListenAndServe(":8080", r))
}
```

## 🎯 Core Concepts
### Request Parameters
Extract parameters from multiple sources using struct tags:
```golang
type GetUserRequest struct {
    // Path parameters
    UserID string `path:"user_id" validate:"required,uuid"`

    // Query parameters
    IncludeDetails bool   `query:"include_details"`
    Page           int    `query:"page" validate:"min=1" default:"1"`

    // Headers
    APIKey string `header:"X-API-Key" validate:"required"`

    // JSON body
    Filters struct {
        Status string `json:"status" validate:"oneof=active inactive"`
    } `json:"filters"`
}
```

### Dependency Injection
Create reusable dependencies that are automatically injected:
```golang
// Define a dependency
type AuthDependency struct{}

type AuthRequest struct {
    Token string `header:"Authorization"`
}

type AuthUser struct {
    UserID   string `json:"user_id"`
    Username string `json:"username"`
}

func (d *AuthDependency) Handle(ctx context.Context, req AuthRequest) (AuthUser, error) {
    // Validate token and return user
    if req.Token == "" {
        return AuthUser{}, gofastapi.NewError(http.StatusUnauthorized, "Token required")
    }
    // Token validation logic...
    return AuthUser{UserID: "123", Username: "john"}, nil
}

// Use in your handlers
type ProtectedRequest struct {
    User AuthUser `dep:"auth"`  // Automatically injected!
    Data string   `json:"data"`
}

func ProtectedHandler(ctx context.Context, req ProtectedRequest) (Response, error) {
    // req.User is automatically populated
    return Response{Message: "Hello " + req.User.Username}, nil
}

// Register the dependency
r.RegisterDependency("auth", &AuthDependency{}, gofastapi.SecuritySchemeBearer)
```

### Error Handling
Built in structured error handling:
```golang
func MyHandler(ctx context.Context, req Request) (Response, error) {
    if !isValid {
        // Return structured errors with status codes
        return Response{}, gofastapi.NewError(http.StatusBadRequest, "Invalid input")
            .WithDetail("field", "value is required")
    }

    // Validation errors are automatically handled
    // Returns 400 with detailed field errors
    return Response{}, nil
}
```

### Groups and Middleware
Organize routes with groups and apply middleware:
```golang
// Create API groups
api := r.Group("/api/v1")
api.Use(rateLimitMiddleware)

// Add routes to groups
api.GET("/users", ListUsers)
api.POST("/users", CreateUser)

// Global middleware
r.Use(loggingMiddleware)
```

### Custom Validators
Add custom validation logic:
```golang
func isEven(fl validator.FieldLevel) bool {
    num := fl.Field().Int()
    return num%2 == 0
}
r.RegisterValidationRule("even", isEven)
```

### OpenAPI Documentation
Automatic OpenAPI 3.0 generation with Scalar UI.
```golang
r := gofastapi.NewWithOpenAPI("My API", "1.0.0", "API Description")

// Add servers
r.AddServer("http://localhost:8080", "Development")
r.AddServer("https://api.example.com", "Production")

// Serve the spec
r.ServeOpenAPIJSON("/openapi.json")
r.ServeDocs("http://localhost:8080", "/docs", nil) // Docs UI available at /docs

// Use struct tags for documentation
type Request struct {
    UserID string `path:"user_id" description:"The user's unique identifier" example:"550e8400-e29b-41d4-a716-446655440000"`
    Name   string `json:"name" description:"User's full name" example:"John Doe"`
}
```

## Real World Example
```golang
package main

import (
    "context"
    "fmt"
    "log"
    "net/http"
    "time"

    "github.com/priyanshu-shubham/gofastapi"
)

// Auth dependency for JWT validation
type AuthDependency struct {
    jwtSecret string
}

func NewAuthDependency(secret string) *AuthDependency {
    return &AuthDependency{jwtSecret: secret}
}

type AuthRequest struct {
    Token string `header:"Authorization"`
}

type AuthUser struct {
    UserID   string   `json:"user_id"`
    Username string   `json:"username"`
    Roles    []string `json:"roles"`
}

func (d *AuthDependency) Handle(ctx context.Context, req AuthRequest) (AuthUser, error) {
    if req.Token == "" {
        return AuthUser{}, gofastapi.NewError(http.StatusUnauthorized, "Missing authorization token")
    }

    // Parse and validate JWT token
    // ... token validation logic ...

    return AuthUser{
        UserID:   "user-123",
        Username: "john.doe",
        Roles:    []string{"admin", "user"},
    }, nil
}

// Rate limiting dependency
type RateLimiter struct {
    store map[string]int
}

type RateLimitRequest struct {
    UserID string `dep:"auth.UserID"`
}

type RateLimitStatus struct {
    Allowed   bool `json:"allowed"`
    Remaining int  `json:"remaining"`
}

func (r *RateLimiter) Handle(ctx context.Context, req RateLimitRequest) (RateLimitStatus, error) {
    // Check rate limits for user
    remaining := 100 - r.store[req.UserID]
    if remaining <= 0 {
        return RateLimitStatus{}, gofastapi.NewError(http.StatusTooManyRequests, "Rate limit exceeded")
    }

    r.store[req.UserID]++
    return RateLimitStatus{
        Allowed:   true,
        Remaining: remaining,
    }, nil
}

// Business logic handlers
type CreatePostRequest struct {
    Auth      AuthUser        `dep:"auth"`
    RateLimit RateLimitStatus `dep:"rate_limit"`

    Title   string   `json:"title" validate:"required,min=3,max=100" description:"Post title"`
    Content string   `json:"content" validate:"required,min=10" description:"Post content"`
    Tags    []string `json:"tags" validate:"max=5" description:"Post tags"`
    Draft   bool     `query:"draft" description:"Save as draft"`
}

type PostResponse struct {
    ID        string    `json:"id"`
    Title     string    `json:"title"`
    Author    string    `json:"author"`
    CreatedAt time.Time `json:"created_at"`
    Draft     bool      `json:"draft"`
}

func CreatePost(ctx context.Context, req CreatePostRequest) (PostResponse, error) {
    // Check permissions
    if !contains(req.Auth.Roles, "author") && !contains(req.Auth.Roles, "admin") {
        return PostResponse{}, gofastapi.NewError(http.StatusForbidden, "Insufficient permissions")
    }

    // Create post in database
    // ... database logic ...

    return PostResponse{
        ID:        fmt.Sprintf("post-%d", time.Now().Unix()),
        Title:     req.Title,
        Author:    req.Auth.Username,
        CreatedAt: time.Now(),
        Draft:     req.Draft,
    }, nil
}

func main() {
    // Create router with OpenAPI
    r := gofastapi.NewWithOpenAPI(
        "Blog API",
        "2.0.0",
        "A comprehensive blog management API",
    )

    // Configure servers
    r.AddServer("http://localhost:8080", "Local development")
    r.AddServer("https://api.blog.com", "Production")

    // Register dependencies
    auth := NewAuthDependency("your-secret-key")
    r.RegisterDependency("auth", auth, gofastapi.SecuritySchemeBearer)
    r.RegisterDependency("rate_limit", &RateLimiter{store: make(map[string]int)})

    // Public routes
    public := r.Group("/api/v1/public")
    public.GET("/health", HealthCheck)

    // Protected routes
    api := r.Group("/api/v1")
    api.POST("/posts", CreatePost)
    api.GET("/posts/{id}", GetPost)
    api.PUT("/posts/{id}", UpdatePost)
    api.DELETE("/posts/{id}", DeletePost)

    // Admin routes
    admin := r.Group("/api/v1/admin")
    admin.GET("/stats", GetStats)
    admin.GET("/users", ListUsers)

    // Serve OpenAPI documentation
    r.ServeOpenAPIJSON("/openapi.json")
    r.ServeDocs("http://localhost:8080", "/docs", nil)

    // Add global middleware
    r.Use(corsMiddleware)
    r.Use(loggingMiddleware)

    log.Println("🚀 Server starting on http://localhost:8080")
    log.Println("📚 OpenAPI Spec at http://localhost:8080/openapi.json")
    log.Println("📖 Docs UI at http://localhost:8080/docs")
    log.Fatal(http.ListenAndServe(":8080", r))
}

func contains(slice []string, item string) bool {
    for _, s := range slice {
        if s == item {
            return true
        }
    }
    return false
}
```

## 📄 License
This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🙏 Acknowledgments
- Inspired by [FastAPI](https://fastapi.tiangolo.com/) for Python.
- Built on top of [Gorilla Mux](https://github.com/gorilla/mux) for routing.
- Uses [go-playground/validator](https://github.com/go-playground/validator) for validation.
- API reference UI by [go-scalar-api-reference](https://github.com/MarceloPetrucio/go-scalar-api-reference).