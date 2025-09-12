package gofastapi

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"sync"
)

// OpenAPI types following OpenAPI 3.0 specification

type OpenAPISpec struct {
	OpenAPI    string               `json:"openapi"`
	Info       OpenAPIInfo          `json:"info"`
	Servers    []OpenAPIServer      `json:"servers,omitempty"`
	Paths      map[string]*PathItem `json:"paths"`
	Components *OpenAPIComponents   `json:"components,omitempty"`
}

type OpenAPIInfo struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Version     string `json:"version"`
}

type OpenAPIServer struct {
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
}

type PathItem struct {
	Get     *Operation `json:"get,omitempty"`
	Post    *Operation `json:"post,omitempty"`
	Put     *Operation `json:"put,omitempty"`
	Patch   *Operation `json:"patch,omitempty"`
	Delete  *Operation `json:"delete,omitempty"`
	Options *Operation `json:"options,omitempty"`
	Head    *Operation `json:"head,omitempty"`
}

type Operation struct {
	OperationID string                 `json:"operationId"`
	Summary     string                 `json:"summary,omitempty"`
	Description string                 `json:"description,omitempty"`
	Tags        []string               `json:"tags,omitempty"`
	Parameters  []Parameter            `json:"parameters,omitempty"`
	RequestBody *RequestBody           `json:"requestBody,omitempty"`
	Responses   map[string]interface{} `json:"responses"` // Can be *Response or *Ref
	Security    []map[string][]string  `json:"security,omitempty"`
	Deprecated  bool                   `json:"deprecated,omitempty"`
}

type Parameter struct {
	Name        string      `json:"name"`
	In          string      `json:"in"` // query, header, path, cookie
	Description string      `json:"description,omitempty"`
	Required    bool        `json:"required"`
	Deprecated  bool        `json:"deprecated,omitempty"`
	Schema      *Schema     `json:"schema"`
	Example     interface{} `json:"example,omitempty"`
}

type RequestBody struct {
	Description string               `json:"description,omitempty"`
	Content     map[string]MediaType `json:"content"`
	Required    bool                 `json:"required"`
}

type Response struct {
	Description string               `json:"description"`
	Content     map[string]MediaType `json:"content,omitempty"`
}

type MediaType struct {
	Schema  *Schema     `json:"schema"`
	Example interface{} `json:"example,omitempty"`
}

// Ref represents a JSON reference
type Ref struct {
	Ref string `json:"$ref"`
}

type Schema struct {
	Type                 string             `json:"type,omitempty"`
	Format               string             `json:"format,omitempty"`
	Title                string             `json:"title,omitempty"`
	Description          string             `json:"description,omitempty"`
	Default              interface{}        `json:"default,omitempty"`
	Example              interface{}        `json:"example,omitempty"`
	Properties           map[string]*Schema `json:"properties,omitempty"`
	Required             []string           `json:"required,omitempty"`
	Items                *Schema            `json:"items,omitempty"`
	AdditionalProperties *Schema            `json:"additionalProperties,omitempty"`
	Minimum              *float64           `json:"minimum,omitempty"`
	Maximum              *float64           `json:"maximum,omitempty"`
	MinLength            *int               `json:"minLength,omitempty"`
	MaxLength            *int               `json:"maxLength,omitempty"`
	MinItems             *int               `json:"minItems,omitempty"`
	MaxItems             *int               `json:"maxItems,omitempty"`
	Enum                 []interface{}      `json:"enum,omitempty"`
	Ref                  string             `json:"$ref,omitempty"`
}

type OpenAPIComponents struct {
	Schemas         map[string]*Schema         `json:"schemas,omitempty"`
	Responses       map[string]*Response       `json:"responses,omitempty"`
	Parameters      map[string]*Parameter      `json:"parameters,omitempty"`
	SecuritySchemes map[string]*SecurityScheme `json:"securitySchemes,omitempty"`
}

// SecuritySchemeType represents the type of security scheme
type SecuritySchemeType string

const (
	SecuritySchemeBearer SecuritySchemeType = "bearer"
	SecuritySchemeBasic  SecuritySchemeType = "basic"
	SecuritySchemeAPIKey SecuritySchemeType = "apiKey"
)

type SecurityScheme struct {
	Type        string `json:"type"` // apiKey, http, oauth2, openIdConnect
	Description string `json:"description,omitempty"`
	Name        string `json:"name,omitempty"`   // for apiKey
	In          string `json:"in,omitempty"`     // for apiKey: header, query, cookie
	Scheme      string `json:"scheme,omitempty"` // for http: basic, bearer
}

// OpenAPIBuilder builds OpenAPI specifications
type OpenAPIBuilder struct {
	spec          *OpenAPISpec
	schemaCache   map[reflect.Type]string // Type -> Schema name in components
	typeProcessor *typeProcessor
	mu            sync.RWMutex
}

type typeProcessor struct {
	processed map[reflect.Type]bool
	schemas   map[string]*Schema
}

func NewOpenAPIBuilder(title, version string) *OpenAPIBuilder {
	return &OpenAPIBuilder{
		spec: &OpenAPISpec{
			OpenAPI: "3.0.3",
			Info: OpenAPIInfo{
				Title:   title,
				Version: version,
			},
			Paths: make(map[string]*PathItem),
			Components: &OpenAPIComponents{
				Schemas:         make(map[string]*Schema),
				Responses:       make(map[string]*Response),
				SecuritySchemes: make(map[string]*SecurityScheme),
			},
		},
		schemaCache: make(map[reflect.Type]string),
		typeProcessor: &typeProcessor{
			processed: make(map[reflect.Type]bool),
			schemas:   make(map[string]*Schema),
		},
	}
}

// SetDescription sets the API description
func (b *OpenAPIBuilder) SetDescription(description string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.spec.Info.Description = description
}

// AddServer adds a server to the spec
func (b *OpenAPIBuilder) AddServer(url, description string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.spec.Servers = append(b.spec.Servers, OpenAPIServer{
		URL:         url,
		Description: description,
	})
}

// AddSecurityScheme adds a security scheme
func (b *OpenAPIBuilder) AddSecurityScheme(schemeType SecuritySchemeType) error {
	scheme := &SecurityScheme{}
	var schemeName string
	switch schemeType {
	case SecuritySchemeBearer:
		scheme.Type = "http"
		scheme.Scheme = "bearer"
		scheme.Description = "Bearer token authentication"
		schemeName = "BearerAuth"
	case SecuritySchemeBasic:
		scheme.Type = "http"
		scheme.Scheme = "basic"
		scheme.Description = "Basic authentication"
		schemeName = "BasicAuth"
	case SecuritySchemeAPIKey:
		scheme.Type = "apiKey"
		scheme.In = "header"
		scheme.Name = "X-API-Key"
		scheme.Description = "API Key authentication via X-API-Key header"
		schemeName = "ApiKeyAuth"
	default:
		return fmt.Errorf("unknown security scheme type: %s", schemeType)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.spec.Components.SecuritySchemes[schemeName] = scheme
	return nil
}

// AddRoute adds a route to the OpenAPI spec
func (b *OpenAPIBuilder) AddRoute(method, path string, handler *CompiledHandler, dependencies []string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Convert gorilla mux path to OpenAPI path
	openAPIPath := convertToOpenAPIPath(path)

	// Get or create path item
	pathItem, exists := b.spec.Paths[openAPIPath]
	if !exists {
		pathItem = &PathItem{}
		b.spec.Paths[openAPIPath] = pathItem
	}

	// Create operation
	operation := b.createOperation(method, openAPIPath, handler, dependencies)

	// Set operation on path item
	switch strings.ToUpper(method) {
	case http.MethodGet:
		pathItem.Get = operation
	case http.MethodPost:
		pathItem.Post = operation
	case http.MethodPut:
		pathItem.Put = operation
	case http.MethodPatch:
		pathItem.Patch = operation
	case http.MethodDelete:
		pathItem.Delete = operation
	case http.MethodOptions:
		pathItem.Options = operation
	case http.MethodHead:
		pathItem.Head = operation
	}
}

// createOperation creates an OpenAPI operation from a compiled handler
func (b *OpenAPIBuilder) createOperation(method, path string, handler *CompiledHandler, dependencies []string) *Operation {
	// Generate operation ID
	operationID := generateOperationID(method, path)

	operation := &Operation{
		OperationID: operationID,
		Parameters:  []Parameter{},
		Responses:   make(map[string]interface{}),
	}

	// Add security requirements if there are dependencies
	if len(dependencies) > 0 {
		operation.Security = []map[string][]string{}
		for _, dep := range dependencies {
			operation.Security = append(operation.Security, map[string][]string{
				dep: {},
			})
		}
	}

	// Extract parameters and request body from request type
	var requestBodySchema *Schema
	var requestBodyRequired []string

	for i := 0; i < handler.reqType.NumField(); i++ {
		field := handler.reqType.Field(i)

		// Skip unexported fields
		if field.PkgPath != "" {
			continue
		}

		// Skip dependency fields
		if depTag := field.Tag.Get("dep"); depTag != "" {
			continue
		}

		// Get validation rules
		validateTag := field.Tag.Get("validate")
		isRequired := strings.Contains(validateTag, "required")

		// Extract description and example from tags
		description := field.Tag.Get("description")
		example := field.Tag.Get("example")
		defaultValue := field.Tag.Get("default")

		// Handle different parameter types
		if pathTag := field.Tag.Get("path"); pathTag != "" {
			param := Parameter{
				Name:        pathTag,
				In:          "path",
				Required:    true, // Path params are always required
				Description: description,
				Schema:      b.createSchemaFromType(field.Type, validateTag),
			}
			if example != "" {
				param.Example = example
			}
			operation.Parameters = append(operation.Parameters, param)
		} else if queryTag := field.Tag.Get("query"); queryTag != "" {
			schema := b.createSchemaFromType(field.Type, validateTag)
			if defaultValue != "" {
				schema.Default = parseValue(defaultValue, field.Type)
			}
			param := Parameter{
				Name:        queryTag,
				In:          "query",
				Required:    isRequired,
				Description: description,
				Schema:      schema,
			}
			if example != "" {
				param.Example = example
			}
			operation.Parameters = append(operation.Parameters, param)
		} else if headerTag := field.Tag.Get("header"); headerTag != "" {
			param := Parameter{
				Name:        headerTag,
				In:          "header",
				Required:    isRequired,
				Description: description,
				Schema:      b.createSchemaFromType(field.Type, validateTag),
			}
			if example != "" {
				param.Example = example
			}
			operation.Parameters = append(operation.Parameters, param)
		} else if jsonTag := field.Tag.Get("json"); jsonTag != "" && jsonTag != "-" {
			// This is part of the request body
			if requestBodySchema == nil {
				requestBodySchema = &Schema{
					Type:       "object",
					Properties: make(map[string]*Schema),
					Required:   []string{},
				}
			}

			fieldName := strings.Split(jsonTag, ",")[0]
			fieldSchema := b.createSchemaFromType(field.Type, validateTag)
			fieldSchema.Description = description
			if example != "" {
				fieldSchema.Example = example
			}
			if defaultValue != "" {
				fieldSchema.Default = parseValue(defaultValue, field.Type)
			}

			requestBodySchema.Properties[fieldName] = fieldSchema
			if isRequired {
				requestBodyRequired = append(requestBodyRequired, fieldName)
			}
		}
	}

	// Add request body if present
	if requestBodySchema != nil {
		requestBodySchema.Required = requestBodyRequired
		operation.RequestBody = &RequestBody{
			Required: len(requestBodyRequired) > 0,
			Content: map[string]MediaType{
				"application/json": {
					Schema: requestBodySchema,
				},
			},
		}
	}

	// Add response schema
	responseSchema := b.getOrCreateSchema(handler.respType)
	operation.Responses["200"] = &Response{
		Description: "Successful response",
		Content: map[string]MediaType{
			"application/json": {
				Schema: responseSchema,
			},
		},
	}

	// Add common error responses
	b.addErrorResponses(operation)

	return operation
}

// createSchemaFromType creates a schema from a Go type
func (b *OpenAPIBuilder) createSchemaFromType(t reflect.Type, validateTag string) *Schema {
	schema := &Schema{}

	// Handle pointers
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	// Map Go types to OpenAPI types
	switch t.Kind() {
	case reflect.String:
		schema.Type = "string"
		// Check for format hints in validation
		if strings.Contains(validateTag, "email") {
			schema.Format = "email"
		} else if strings.Contains(validateTag, "uuid") {
			schema.Format = "uuid"
		} else if strings.Contains(validateTag, "uri") {
			schema.Format = "uri"
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32:
		schema.Type = "integer"
		schema.Format = "int32"
	case reflect.Int64:
		schema.Type = "integer"
		schema.Format = "int64"
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32:
		schema.Type = "integer"
		schema.Format = "int32"
	case reflect.Uint64:
		schema.Type = "integer"
		schema.Format = "int64"
	case reflect.Float32:
		schema.Type = "number"
		schema.Format = "float"
	case reflect.Float64:
		schema.Type = "number"
		schema.Format = "double"
	case reflect.Bool:
		schema.Type = "boolean"
	case reflect.Slice, reflect.Array:
		if t.String() == "uuid.UUID" {
			schema.Type = "string"
			schema.Format = "uuid"
		} else {
			schema.Type = "array"
			schema.Items = b.createSchemaFromType(t.Elem(), "")
		}
	case reflect.Struct:
		// Handle time.Time specially
		if t.String() == "time.Time" {
			schema.Type = "string"
			schema.Format = "date-time"
		} else {
			// Reference to component schema
			return b.getOrCreateSchema(t)
		}
	case reflect.Map:
		schema.Type = "object"
		schema.AdditionalProperties = b.createSchemaFromType(t.Elem(), "")
	default:
		schema.Type = "string" // Default fallback
	}

	// Apply validation constraints
	b.applyValidationConstraints(schema, validateTag)

	return schema
}

// getOrCreateSchema gets or creates a schema in components
func (b *OpenAPIBuilder) getOrCreateSchema(t reflect.Type) *Schema {
	// Check cache
	if schemaName, exists := b.schemaCache[t]; exists {
		return &Schema{Ref: "#/components/schemas/" + schemaName}
	}

	// Generate schema name
	schemaName := t.Name()
	if schemaName == "" {
		schemaName = "Schema" + fmt.Sprintf("%d", len(b.spec.Components.Schemas))
	}

	// Create the schema
	schema := &Schema{
		Type:       "object",
		Properties: make(map[string]*Schema),
		Required:   []string{},
	}

	// Process struct fields
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Skip unexported fields
		if field.PkgPath != "" {
			continue
		}

		// Get JSON tag
		jsonTag := field.Tag.Get("json")
		if jsonTag == "-" {
			continue
		}

		fieldName := field.Name
		if jsonTag != "" {
			fieldName = strings.Split(jsonTag, ",")[0]
		}

		// Get validation rules
		validateTag := field.Tag.Get("validate")
		isRequired := strings.Contains(validateTag, "required")

		// Create field schema
		fieldSchema := b.createSchemaFromType(field.Type, validateTag)

		// Add description and example if present
		if desc := field.Tag.Get("description"); desc != "" {
			fieldSchema.Description = desc
		}
		if example := field.Tag.Get("example"); example != "" {
			fieldSchema.Example = example
		}

		schema.Properties[fieldName] = fieldSchema
		if isRequired {
			schema.Required = append(schema.Required, fieldName)
		}
	}

	// Store in components and cache
	b.spec.Components.Schemas[schemaName] = schema
	b.schemaCache[t] = schemaName

	// Return reference
	return &Schema{Ref: "#/components/schemas/" + schemaName}
}

// applyValidationConstraints applies validation constraints to a schema
func (b *OpenAPIBuilder) applyValidationConstraints(schema *Schema, validateTag string) {
	if validateTag == "" {
		return
	}

	parts := strings.Split(validateTag, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)

		// Handle min/max constraints
		if strings.HasPrefix(part, "min=") {
			if val := parseIntConstraint(part[4:]); val != nil {
				if schema.Type == "string" {
					schema.MinLength = val
				} else if schema.Type == "array" {
					schema.MinItems = val
				} else if schema.Type == "number" || schema.Type == "integer" {
					floatVal := float64(*val)
					schema.Minimum = &floatVal
				}
			}
		} else if strings.HasPrefix(part, "max=") {
			if val := parseIntConstraint(part[4:]); val != nil {
				if schema.Type == "string" {
					schema.MaxLength = val
				} else if schema.Type == "array" {
					schema.MaxItems = val
				} else if schema.Type == "number" || schema.Type == "integer" {
					floatVal := float64(*val)
					schema.Maximum = &floatVal
				}
			}
		}
	}
}

// addErrorResponses adds common error responses
func (b *OpenAPIBuilder) addErrorResponses(operation *Operation) {
	// Add 400 Bad Request
	if _, exists := b.spec.Components.Responses["ValidationError"]; !exists {
		b.spec.Components.Responses["ValidationError"] = &Response{
			Description: "Validation error",
			Content: map[string]MediaType{
				"application/json": {
					Schema: &Schema{
						Type: "object",
						Properties: map[string]*Schema{
							"code":    {Type: "string"},
							"message": {Type: "string"},
							"validation_errors": {
								Type: "object",
								AdditionalProperties: &Schema{
									Type:  "array",
									Items: &Schema{Type: "string"},
								},
							},
						},
					},
				},
			},
		}
	}
	// Use Ref type for reference
	operation.Responses["400"] = &Ref{Ref: "#/components/responses/ValidationError"}

	// Add 401 Unauthorized if security is required
	if len(operation.Security) > 0 {
		if _, exists := b.spec.Components.Responses["UnauthorizedError"]; !exists {
			b.spec.Components.Responses["UnauthorizedError"] = &Response{
				Description: "Authentication required",
				Content: map[string]MediaType{
					"application/json": {
						Schema: &Schema{
							Type: "object",
							Properties: map[string]*Schema{
								"code":    {Type: "string"},
								"message": {Type: "string"},
							},
						},
					},
				},
			}
		}
		// Use Ref type for reference
		operation.Responses["401"] = &Ref{Ref: "#/components/responses/UnauthorizedError"}
	}

	// Add 500 Internal Server Error
	if _, exists := b.spec.Components.Responses["InternalError"]; !exists {
		b.spec.Components.Responses["InternalError"] = &Response{
			Description: "Internal server error",
			Content: map[string]MediaType{
				"application/json": {
					Schema: &Schema{
						Type: "object",
						Properties: map[string]*Schema{
							"code":    {Type: "string"},
							"message": {Type: "string"},
						},
					},
				},
			},
		}
	}
	// Use Ref type for reference
	operation.Responses["500"] = &Ref{Ref: "#/components/responses/InternalError"}
}

// GetSpec returns the built OpenAPI spec
func (b *OpenAPIBuilder) GetSpec() *OpenAPISpec {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.spec
}

// Helper functions

func convertToOpenAPIPath(path string) string {
	// Convert gorilla mux {param} to OpenAPI {param}
	// The formats are already compatible!
	return path
}

func generateOperationID(method, path string) string {
	// Generate operation ID from method and path
	// e.g., POST /categories/{category_id}/posts -> postCategoriesCategoryIdPosts

	parts := strings.Split(path, "/")
	var words []string

	words = append(words, strings.ToLower(method))

	for _, part := range parts {
		if part == "" {
			continue
		}
		// Remove curly braces and convert to camelCase
		part = strings.Trim(part, "{}")
		part = strings.ReplaceAll(part, "_", " ")
		part = strings.ReplaceAll(part, "-", " ")
		words = append(words, toCamelCase(part))
	}

	return strings.Join(words, "")
}

func toCamelCase(s string) string {
	words := strings.Fields(s)
	for i, word := range words {
		if i == 0 {
			words[i] = strings.ToLower(word)
		} else {
			words[i] = strings.Title(strings.ToLower(word))
		}
	}
	return strings.Join(words, "")
}

func parseIntConstraint(s string) *int {
	var val int
	if _, err := fmt.Sscanf(s, "%d", &val); err == nil {
		return &val
	}
	return nil
}

func parseValue(s string, t reflect.Type) interface{} {
	switch t.Kind() {
	case reflect.Bool:
		return s == "true"
	case reflect.String:
		return s
	case reflect.Int, reflect.Int32, reflect.Int64:
		var val int
		fmt.Sscanf(s, "%d", &val)
		return val
	case reflect.Float32, reflect.Float64:
		var val float64
		fmt.Sscanf(s, "%f", &val)
		return val
	default:
		return s
	}
}
