package gofastapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/MarceloPetrucio/go-scalar-api-reference"
	"github.com/go-playground/validator/v10"
	"github.com/gorilla/mux"
)

// Router is the main router struct
type Router struct {
	mux            *mux.Router
	routes         map[string]*CompiledHandler
	routeMetadata  map[string]*routeInfo
	depResolver    *DependencyResolver
	errorHandler   ErrorHandler
	middleware     []mux.MiddlewareFunc
	openAPIBuilder *OpenAPIBuilder
	mu             sync.RWMutex
	openapiJSONURL *string
}

type routeInfo struct {
	method       string
	path         string
	handler      *CompiledHandler
	dependencies []string
}

// New creates a new router instance
func New() *Router {
	return &Router{
		mux:            mux.NewRouter(),
		routes:         make(map[string]*CompiledHandler),
		routeMetadata:  make(map[string]*routeInfo),
		depResolver:    NewDependencyResolver(),
		errorHandler:   defaultErrorHandler,
		openAPIBuilder: NewOpenAPIBuilder("API", "1.0.0"),
	}
}

// NewWithOpenAPI creates a new router with OpenAPI configuration
func NewWithOpenAPI(title, version, description string) *Router {
	r := New()
	r.openAPIBuilder = NewOpenAPIBuilder(title, version)
	if description != "" {
		r.openAPIBuilder.SetDescription(description)
	}
	return r
}

// RegisterDependency registers a dependency for injection
func (r *Router) RegisterDependency(name string, dep interface{}, schemeTypes ...SecuritySchemeType) error {
	err := r.depResolver.Register(name, dep)
	if err != nil {
		return err
	}
	for _, schemeType := range schemeTypes {
		err := r.openAPIBuilder.AddSecurityScheme(schemeType)
		if err != nil {
			return err
		}
	}
	return nil
}

// RegisterValidationRule adds a new validation rule to the underlying validator.
func (r *Router) RegisterValidationRule(tag string, fn validator.Func) error {
	return addValidationRule(tag, fn)
}

// SetErrorHandler sets a custom error handler
func (r *Router) SetErrorHandler(handler ErrorHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.errorHandler = handler
}

// Use adds middleware to the router
func (r *Router) Use(middleware ...mux.MiddlewareFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.middleware = append(r.middleware, middleware...)
	r.mux.Use(middleware...)
}

// ServeHTTP implements http.Handler
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mux.ServeHTTP(w, req)
}

// GET registers a GET route
func (r *Router) GET(path string, handler interface{}) error {
	return r.registerRoute(http.MethodGet, path, handler)
}

// POST registers a POST route
func (r *Router) POST(path string, handler interface{}) error {
	return r.registerRoute(http.MethodPost, path, handler)
}

// PUT registers a PUT route
func (r *Router) PUT(path string, handler interface{}) error {
	return r.registerRoute(http.MethodPut, path, handler)
}

// PATCH registers a PATCH route
func (r *Router) PATCH(path string, handler interface{}) error {
	return r.registerRoute(http.MethodPatch, path, handler)
}

// DELETE registers a DELETE route
func (r *Router) DELETE(path string, handler interface{}) error {
	return r.registerRoute(http.MethodDelete, path, handler)
}

// registerRoute compiles and registers a route handler
func (r *Router) registerRoute(method, path string, handler interface{}) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Compile the handler
	compiled, err := compileHandler(handler)
	if err != nil {
		return fmt.Errorf("failed to compile handler for %s %s: %w", method, path, err)
	}

	// Extract dependencies from the handler
	var dependencies []string
	for _, depName := range compiled.dependencies {
		dependencies = append(dependencies, depName)
	}

	// Store compiled handler and metadata
	routeKey := fmt.Sprintf("%s:%s", method, path)
	r.routes[routeKey] = compiled
	r.routeMetadata[routeKey] = &routeInfo{
		method:       method,
		path:         path,
		handler:      compiled,
		dependencies: dependencies,
	}

	// Add to OpenAPI spec
	r.openAPIBuilder.AddRoute(method, path, compiled, dependencies)

	// Register with mux
	r.mux.HandleFunc(path, func(w http.ResponseWriter, req *http.Request) {
		// Get the compiled handler
		r.mu.RLock()
		handler := r.routes[routeKey]
		errorHandler := r.errorHandler
		r.mu.RUnlock()

		if handler == nil {
			errorHandler(w, req, NewError(http.StatusNotFound, "Route not found"))
			return
		}

		// Execute the compiled handler with the error handler
		ctx := req.Context()
		handler.Execute(ctx, w, req, r.depResolver, errorHandler)
	}).Methods(method)

	return nil
}

// Group creates a subrouter with a prefix
func (r *Router) Group(prefix string) *SubRouter {
	return &SubRouter{
		router: r,
		prefix: prefix,
		mux:    r.mux.PathPrefix(prefix).Subrouter(),
	}
}

// SSEGET registers an SSE GET route
func (r *Router) SSEGET(path string, handler interface{}) error {
	return r.registerSSERoute(http.MethodGet, path, handler)
}

// SSEPOST registers an SSE POST route
func (r *Router) SSEPOST(path string, handler interface{}) error {
	return r.registerSSERoute(http.MethodPost, path, handler)
}

// registerSSERoute compiles and registers an SSE route handler
func (r *Router) registerSSERoute(method, path string, handler interface{}) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Compile the SSE handler
	compiled, err := compileSSEHandler(handler)
	if err != nil {
		return fmt.Errorf("failed to compile SSE handler for %s %s: %w", method, path, err)
	}

	// Extract dependencies
	var dependencies []string
	for _, depName := range compiled.dependencies {
		dependencies = append(dependencies, depName)
	}

	// Store metadata (reuse existing routeInfo structure)
	routeKey := fmt.Sprintf("%s:%s", method, path)
	r.routeMetadata[routeKey] = &routeInfo{
		method:       method,
		path:         path,
		handler:      nil, // SSE handlers don't use regular CompiledHandler
		dependencies: dependencies,
	}

	// Add to OpenAPI spec
	r.openAPIBuilder.AddSSERoute(method, path, compiled, dependencies)

	// Register with mux
	r.mux.HandleFunc(path, func(w http.ResponseWriter, req *http.Request) {
		r.mu.RLock()
		errorHandler := r.errorHandler
		r.mu.RUnlock()

		// Execute the compiled SSE handler
		ctx := req.Context()
		compiled.Execute(ctx, w, req, r.depResolver, errorHandler)
	}).Methods(method)

	return nil
}

// SubRouter represents a group of routes with a common prefix
type SubRouter struct {
	router *Router
	prefix string
	mux    *mux.Router
}

// GET registers a GET route in the group
func (sr *SubRouter) GET(path string, handler interface{}) error {
	fullPath := sr.prefix + path
	return sr.router.registerRoute(http.MethodGet, fullPath, handler)
}

// POST registers a POST route in the group
func (sr *SubRouter) POST(path string, handler interface{}) error {
	fullPath := sr.prefix + path
	return sr.router.registerRoute(http.MethodPost, fullPath, handler)
}

// PUT registers a PUT route in the group
func (sr *SubRouter) PUT(path string, handler interface{}) error {
	fullPath := sr.prefix + path
	return sr.router.registerRoute(http.MethodPut, fullPath, handler)
}

// PATCH registers a PATCH route in the group
func (sr *SubRouter) PATCH(path string, handler interface{}) error {
	fullPath := sr.prefix + path
	return sr.router.registerRoute(http.MethodPatch, fullPath, handler)
}

// DELETE registers a DELETE route in the group
func (sr *SubRouter) DELETE(path string, handler interface{}) error {
	fullPath := sr.prefix + path
	return sr.router.registerRoute(http.MethodDelete, fullPath, handler)
}

// Use adds middleware to the subrouter
func (sr *SubRouter) Use(middleware ...mux.MiddlewareFunc) {
	sr.mux.Use(middleware...)
}

func (sr *SubRouter) SSEGET(path string, handler interface{}) error {
	fullPath := sr.prefix + path
	return sr.router.registerSSERoute(http.MethodGet, fullPath, handler)
}

func (sr *SubRouter) SSEPOST(path string, handler interface{}) error {
	fullPath := sr.prefix + path
	return sr.router.registerSSERoute(http.MethodPost, fullPath, handler)
}

// GenerateOpenAPISpec returns the OpenAPI specification
func (r *Router) GenerateOpenAPISpec() *OpenAPISpec {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.openAPIBuilder.GetSpec()
}

// ServeOpenAPIJSON serves the OpenAPI spec as JSON at the specified path
func (r *Router) ServeOpenAPIJSON(path string) {
	r.mux.HandleFunc(path, func(w http.ResponseWriter, req *http.Request) {
		spec := r.GenerateOpenAPISpec()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*") // For Swagger UI
		json.NewEncoder(w).Encode(spec)
	}).Methods(http.MethodGet)
	r.openapiJSONURL = &path
}

// ServeDocs serves the OpenAPI spec as HTML doc at the specified path
func (r *Router) ServeDocs(baseURL string, path string, options *scalar.Options) {
	if r.openapiJSONURL == nil {
		r.ServeOpenAPIJSON("/openapi.json")
	}
	if options == nil {
		openAPISpec := r.GenerateOpenAPISpec()
		options = &scalar.Options{
			SpecURL: baseURL + *r.openapiJSONURL,
			Theme:   scalar.ThemeKepler,
			CustomOptions: scalar.CustomOptions{
				PageTitle: openAPISpec.Info.Title + " Documentation",
			},
			DarkMode: true,
		}
	}
	r.mux.HandleFunc(path, func(w http.ResponseWriter, req *http.Request) {
		html, err := scalar.ApiReferenceHTML(options)
		if err != nil {
			http.Error(w, "Failed to generate API reference HTML", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(html))
	}).Methods(http.MethodGet)
}

// AddServer adds a server to the OpenAPI spec
func (r *Router) AddServer(url, description string) {
	r.openAPIBuilder.AddServer(url, description)
}
