package gofastapi

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"sync"
)

// Dependency represents a dependency that can be injected
type Dependency interface {
	// The Handle method will be detected via reflection
}

// DependencyResolver manages dependency resolution
type DependencyResolver struct {
	dependencies map[string]*compiledDependency
	mu           sync.RWMutex
}

type compiledDependency struct {
	instance    interface{}
	handlerFunc reflect.Value
	reqType     reflect.Type
	respType    reflect.Type
	extractors  map[int]FieldExtractor
	validators  map[int]string
}

// ResolvedDependencies holds resolved dependency values for a request
type ResolvedDependencies struct {
	values map[string]interface{}
	mu     sync.Mutex
}

// DependencyError wraps errors that occur during dependency resolution
type DependencyError struct {
	DependencyName string
	Err            error
}

func (e *DependencyError) Error() string {
	return fmt.Sprintf("dependency '%s' failed: %v", e.DependencyName, e.Err)
}

func (e *DependencyError) Unwrap() error {
	return e.Err
}

func NewDependencyResolver() *DependencyResolver {
	return &DependencyResolver{
		dependencies: make(map[string]*compiledDependency),
	}
}

// Register compiles and registers a dependency
func (dr *DependencyResolver) Register(name string, dep interface{}) error {
	dr.mu.Lock()
	defer dr.mu.Unlock()

	depValue := reflect.ValueOf(dep)

	// Find the Handle method
	handleMethod := depValue.MethodByName("Handle")
	if !handleMethod.IsValid() {
		return fmt.Errorf("dependency must have a Handle method")
	}

	handleType := handleMethod.Type()
	if handleType.NumIn() != 2 || handleType.NumOut() != 2 {
		return fmt.Errorf("`Handle` method must have signature: Handle(context.Context, Request) (Response, error)")
	}

	// Verify first param is context.Context
	if handleType.In(0) != reflect.TypeOf((*context.Context)(nil)).Elem() {
		return fmt.Errorf("first parameter must be context.Context")
	}

	// Get request and response types
	reqType := handleType.In(1)
	respType := handleType.Out(0)

	// Verify second return type is error
	if handleType.Out(1) != reflect.TypeOf((*error)(nil)).Elem() {
		return fmt.Errorf("second return value must be error")
	}

	// Compile extractors for the request struct
	extractors, validators, err := compileStructExtractors(reqType)
	if err != nil {
		return fmt.Errorf("failed to compile dependency extractors: %w", err)
	}

	dr.dependencies[name] = &compiledDependency{
		instance:    dep,
		handlerFunc: handleMethod,
		reqType:     reqType,
		respType:    respType,
		extractors:  extractors,
		validators:  validators,
	}

	return nil
}

// Resolve executes a dependency and caches the result
func (dr *DependencyResolver) Resolve(ctx context.Context, name string, r *http.Request, vars map[string]string, body []byte, resolved *ResolvedDependencies) (interface{}, error) {
	// Check if already resolved
	resolved.mu.Lock()
	if val, ok := resolved.values[name]; ok {
		resolved.mu.Unlock()
		return val, nil
	}
	resolved.mu.Unlock()

	dr.mu.RLock()
	dep, ok := dr.dependencies[name]
	dr.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("dependency %s not found", name)
	}

	// Create request struct
	reqValue := reflect.New(dep.reqType).Elem()

	// Extract fields
	for fieldIdx, extractor := range dep.extractors {
		var value interface{}
		var err error

		// Handle dependency extractors specially
		if depExt, ok := extractor.(*DependencyExtractor); ok {
			// Resolve the dependency first
			depResult, err := dr.Resolve(ctx, depExt.depName, r, vars, body, resolved)
			if err != nil {
				// Propagate the error as-is if it's already a known error type
				return nil, err
			}

			// Extract nested field if needed
			if len(depExt.fieldPath) > 1 {
				value = extractNestedField(depResult, depExt.fieldPath[1:]) // Skip the dep name
			} else {
				value = depResult
			}
		} else {
			value, err = extractor.Extract(r, vars, body)
			if err != nil {
				return nil, err
			}
		}

		if value != nil {
			field := reqValue.Field(fieldIdx)
			fieldValue := reflect.ValueOf(value)
			if fieldValue.Type().ConvertibleTo(field.Type()) {
				field.Set(fieldValue.Convert(field.Type()))
			}
		}
	}

	// Validate the request - this returns ValidationError which we need to preserve
	if err := validateStruct(reqValue.Interface(), dep.validators); err != nil {
		// Don't wrap validation errors, return them as-is
		return nil, err
	}

	// Call the dependency handler
	results := dep.handlerFunc.Call([]reflect.Value{
		reflect.ValueOf(ctx),
		reqValue,
	})

	if !results[1].IsNil() {
		// Return the error from the handler as-is to preserve its type
		return nil, results[1].Interface().(error)
	}

	result := results[0].Interface()

	// Cache the result
	resolved.mu.Lock()
	resolved.values[name] = result
	resolved.mu.Unlock()

	return result, nil
}

// extractNestedField extracts a nested field from a struct
func extractNestedField(obj interface{}, path []string) interface{} {
	if len(path) == 0 {
		return obj
	}

	v := reflect.ValueOf(obj)
	for _, field := range path {
		// Handle pointers
		for v.Kind() == reflect.Ptr {
			if v.IsNil() {
				return nil
			}
			v = v.Elem()
		}

		if v.Kind() != reflect.Struct {
			return nil
		}

		// Try to find the field (case-sensitive first)
		fieldValue := v.FieldByName(field)

		// If not found, try case-insensitive search
		if !fieldValue.IsValid() {
			t := v.Type()
			for i := 0; i < v.NumField(); i++ {
				if strings.EqualFold(t.Field(i).Name, field) {
					fieldValue = v.Field(i)
					break
				}
			}
		}

		if !fieldValue.IsValid() {
			return nil
		}
		v = fieldValue
	}
	return v.Interface()
}
