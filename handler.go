package gofastapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"

	"github.com/gorilla/mux"
)

// HandlerFunc is the signature for route handlers
type HandlerFunc interface{}

// CompiledHandler represents a pre-compiled handler
type CompiledHandler struct {
	handlerFunc  reflect.Value
	reqType      reflect.Type
	respType     reflect.Type
	extractors   map[int]FieldExtractor
	validators   map[int]string
	dependencies map[int]string // field index -> dependency name
	hasJSONBody  bool
}

// compileHandler pre-compiles a handler function for efficient execution
func compileHandler(handler interface{}) (*CompiledHandler, error) {
	handlerType := reflect.TypeOf(handler)
	handlerValue := reflect.ValueOf(handler)

	// Validate handler signature
	if handlerType.Kind() != reflect.Func {
		return nil, fmt.Errorf("handler must be a function")
	}

	if handlerType.NumIn() != 2 || handlerType.NumOut() != 2 {
		return nil, fmt.Errorf("handler must have signature: func(context.Context, Request) (Response, error)")
	}

	// Verify first param is context.Context
	if handlerType.In(0) != reflect.TypeOf((*context.Context)(nil)).Elem() {
		return nil, fmt.Errorf("first parameter must be context.Context")
	}

	// Verify second return type is error
	if handlerType.Out(1) != reflect.TypeOf((*error)(nil)).Elem() {
		return nil, fmt.Errorf("second return value must be error")
	}

	reqType := handlerType.In(1)
	respType := handlerType.Out(0)

	extractors, validators, err := compileStructExtractors(reqType)
	if err != nil {
		return nil, err
	}

	// Check for dependencies and JSON body
	dependencies := make(map[int]string)
	hasJSONBody := false

	for i := 0; i < reqType.NumField(); i++ {
		field := reqType.Field(i)

		if depTag := field.Tag.Get("dep"); depTag != "" {
			dependencies[i] = strings.Split(depTag, ".")[0]
		}

		if jsonTag := field.Tag.Get("json"); jsonTag != "" && jsonTag != "-" {
			hasJSONBody = true
		}
	}

	return &CompiledHandler{
		handlerFunc:  handlerValue,
		reqType:      reqType,
		respType:     respType,
		extractors:   extractors,
		validators:   validators,
		dependencies: dependencies,
		hasJSONBody:  hasJSONBody,
	}, nil
}

// compileStructExtractors creates extractors for all fields in a struct
func compileStructExtractors(structType reflect.Type) (map[int]FieldExtractor, map[int]string, error) {
	extractors := make(map[int]FieldExtractor)
	validators := make(map[int]string)

	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)

		// Skip unexported fields
		if field.PkgPath != "" {
			continue
		}

		// Handle different tag types
		if pathTag := field.Tag.Get("path"); pathTag != "" {
			extractors[i] = &PathExtractor{
				paramName: pathTag,
				fieldType: field.Type,
			}
		} else if queryTag := field.Tag.Get("query"); queryTag != "" {
			extractors[i] = &QueryExtractor{
				paramName: queryTag,
				fieldType: field.Type,
			}
		} else if headerTag := field.Tag.Get("header"); headerTag != "" {
			extractors[i] = &HeaderExtractor{
				headerName: headerTag,
				fieldType:  field.Type,
			}
		} else if jsonTag := field.Tag.Get("json"); jsonTag != "" && jsonTag != "-" {
			jsonPath := strings.Split(jsonTag, ",")[0]
			extractors[i] = &JSONExtractor{
				jsonPath:  jsonPath,
				fieldType: field.Type,
			}
		} else if depTag := field.Tag.Get("dep"); depTag != "" {
			parts := strings.Split(depTag, ".")
			extractors[i] = &DependencyExtractor{
				depName:   parts[0],
				fieldPath: parts,
				fieldType: field.Type,
			}
		}

		// Store validation tags
		if validateTag := field.Tag.Get("validate"); validateTag != "" {
			validators[i] = validateTag
		}
	}

	return extractors, validators, nil
}

// Execute runs the compiled handler
func (ch *CompiledHandler) Execute(ctx context.Context, w http.ResponseWriter, r *http.Request, depResolver *DependencyResolver, errorHandler ErrorHandler) {
	// Read body once if needed
	var body []byte
	var err error
	if ch.hasJSONBody && r.Body != nil {
		body, err = io.ReadAll(r.Body)
		if err != nil {
			errorHandler(w, r, fmt.Errorf("failed to read request body: %w", err))
			return
		}
		defer r.Body.Close()
	}

	// Extract path variables
	vars := mux.Vars(r)

	// Create request struct
	reqValue := reflect.New(ch.reqType).Elem()

	// Initialize resolved dependencies
	resolved := &ResolvedDependencies{
		values: make(map[string]interface{}),
	}

	// Extract all fields
	for fieldIdx, extractor := range ch.extractors {
		var value interface{}

		// Handle dependency extractors
		if depExt, ok := extractor.(*DependencyExtractor); ok {
			// Resolve the dependency
			depResult, err := depResolver.Resolve(ctx, depExt.depName, r, vars, body, resolved)
			if err != nil {
				// Pass the error directly to preserve its type
				errorHandler(w, r, err)
				return
			}

			// Extract nested field if needed
			if len(depExt.fieldPath) > 1 {
				value = extractNestedField(depResult, depExt.fieldPath[1:])
			} else {
				value = depResult
			}
		} else {
			value, err = extractor.Extract(r, vars, body)
			if err != nil {
				errorHandler(w, r, err)
				return
			}
		}

		if value != nil {
			field := reqValue.Field(fieldIdx)
			fieldValue := reflect.ValueOf(value)

			// Handle type conversion
			if fieldValue.Type().ConvertibleTo(field.Type()) {
				field.Set(fieldValue.Convert(field.Type()))
			} else {
				field.Set(fieldValue)
			}
		}
	}

	// Validate the request
	if err := validateStruct(reqValue.Interface(), ch.validators); err != nil {
		errorHandler(w, r, err)
		return
	}

	// Call the handler
	results := ch.handlerFunc.Call([]reflect.Value{
		reflect.ValueOf(ctx),
		reqValue,
	})

	// Handle error response
	if !results[1].IsNil() {
		errorHandler(w, r, results[1].Interface().(error))
		return
	}

	// Serialize response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(results[0].Interface()); err != nil {
		// Log error but response is already partially written
		fmt.Printf("Failed to encode response: %v\n", err)
	}
}
