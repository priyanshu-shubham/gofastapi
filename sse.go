package gofastapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
)

// EventData represents a Server-Sent Event
type EventData[T any] struct {
	Event string `json:"-"`    // SSE event field (optional)
	Data  T      `json:"data"` // The actual data payload (required)
	ID    string `json:"-"`    // SSE id field (optional)
	Retry int    `json:"-"`    // SSE retry field in milliseconds (optional)
}

// SSECompiledHandler represents a pre-compiled SSE handler
type SSECompiledHandler struct {
	handlerFunc  reflect.Value
	reqType      reflect.Type
	respType     reflect.Type // The T in iter.Seq[EventData[T]]
	extractors   map[int]FieldExtractor
	validators   map[int]string
	dependencies map[int]string
	hasJSONBody  bool
}

// compileSSEHandler pre-compiles an SSE handler function
func compileSSEHandler(handler interface{}) (*SSECompiledHandler, error) {
	handlerType := reflect.TypeOf(handler)
	handlerValue := reflect.ValueOf(handler)

	// Validate handler signature
	if handlerType.Kind() != reflect.Func {
		return nil, fmt.Errorf("handler must be a function")
	}

	if handlerType.NumIn() != 2 || handlerType.NumOut() != 2 {
		return nil, fmt.Errorf("SSE handler must have signature: func(context.Context, Request) (iter.Seq[EventData[T]], error)")
	}

	// Verify first param is context.Context
	if handlerType.In(0) != reflect.TypeOf((*context.Context)(nil)).Elem() {
		return nil, fmt.Errorf("first parameter must be context.Context")
	}

	// Verify second return type is error
	if handlerType.Out(1) != reflect.TypeOf((*error)(nil)).Elem() {
		return nil, fmt.Errorf("second return value must be error")
	}

	// Verify first return type is iter.Seq[EventData[T]]
	iterType := handlerType.Out(0)
	if !isIterSeqOfEventData(iterType) {
		return nil, fmt.Errorf("first return value must be iter.Seq[EventData[T]]")
	}

	// Extract the T type from iter.Seq[EventData[T]]
	respType := extractEventDataType(iterType)
	if respType == nil {
		return nil, fmt.Errorf("could not extract event data type from iterator")
	}

	reqType := handlerType.In(1)

	// Reuse existing compilation logic
	extractors, validators, err := compileStructExtractors(reqType)
	if err != nil {
		return nil, err
	}

	// Check for dependencies and JSON body
	dependencies, hasJSONBody := extractHandlerMetadata(reqType)

	return &SSECompiledHandler{
		handlerFunc:  handlerValue,
		reqType:      reqType,
		respType:     respType,
		extractors:   extractors,
		validators:   validators,
		dependencies: dependencies,
		hasJSONBody:  hasJSONBody,
	}, nil
}

// Execute runs the compiled SSE handler
func (sh *SSECompiledHandler) Execute(ctx context.Context, w http.ResponseWriter, r *http.Request, depResolver *DependencyResolver, errorHandler ErrorHandler) {
	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Cache-Control")

	// Get the request struct using shared logic
	reqValue, _, err := sh.prepareRequest(ctx, r, depResolver)
	if err != nil {
		errorHandler(w, r, err)
		return
	}

	// Call the handler
	results := sh.handlerFunc.Call([]reflect.Value{
		reflect.ValueOf(ctx),
		reqValue,
	})

	// Handle error response
	if !results[1].IsNil() {
		errorHandler(w, r, results[1].Interface().(error))
		return
	}

	// Get the iterator
	iterValue := results[0]
	if iterValue.IsNil() {
		errorHandler(w, r, fmt.Errorf("handler returned nil iterator"))
		return
	}

	// Start streaming
	sh.streamEvents(ctx, w, iterValue)
}

// prepareRequest prepares the request struct
func (sh *SSECompiledHandler) prepareRequest(ctx context.Context, r *http.Request, depResolver *DependencyResolver) (reflect.Value, *ResolvedDependencies, error) {
	// Read body once if needed
	var body []byte
	var err error
	if sh.hasJSONBody && r.Body != nil {
		body, err = io.ReadAll(r.Body)
		if err != nil {
			return reflect.Value{}, nil, fmt.Errorf("failed to read request body: %w", err)
		}
		defer r.Body.Close()
	}

	// Extract path variables
	vars := getPathVars(r)

	// Create request struct
	reqValue := reflect.New(sh.reqType).Elem()

	// Initialize resolved dependencies
	resolved := &ResolvedDependencies{
		values: make(map[string]interface{}),
	}

	// Extract all fields using shared logic
	err = extractFields(ctx, reqValue, sh.extractors, sh.dependencies, r, vars, body, depResolver, resolved)
	if err != nil {
		return reflect.Value{}, nil, err
	}

	// Validate the request
	if err := validateStruct(reqValue.Interface(), sh.validators); err != nil {
		return reflect.Value{}, nil, err
	}

	return reqValue, resolved, nil
}

// streamEvents handles the actual SSE streaming
func (sh *SSECompiledHandler) streamEvents(ctx context.Context, w http.ResponseWriter, iterValue reflect.Value) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Create a yield function using reflection
	// The iterator expects: func(yield func(EventData[T]) bool)
	// We need to create: func(EventData[T]) bool

	yieldFuncType := iterValue.Type().In(0) // This is func(EventData[T]) bool
	yieldFunc := reflect.MakeFunc(yieldFuncType, func(args []reflect.Value) []reflect.Value {
		// Check if context is still valid
		select {
		case <-ctx.Done():
			return []reflect.Value{reflect.ValueOf(false)}
		default:
		}

		if len(args) != 1 {
			return []reflect.Value{reflect.ValueOf(false)}
		}

		eventData := args[0].Interface()

		// Write the SSE event
		if err := sh.writeSSEEvent(w, eventData); err != nil {
			fmt.Printf("Error writing SSE event: %v\n", err)
			return []reflect.Value{reflect.ValueOf(false)}
		}

		flusher.Flush()
		return []reflect.Value{reflect.ValueOf(true)}
	})

	// Call the iterator: iter(yieldFunc)
	iterValue.Call([]reflect.Value{yieldFunc})
}

// writeSSEEvent writes a single SSE event
func (sh *SSECompiledHandler) writeSSEEvent(w io.Writer, eventInterface interface{}) error {
	// Use reflection to extract EventData fields safely
	eventValue := reflect.ValueOf(eventInterface)
	if eventValue.Kind() == reflect.Ptr {
		eventValue = eventValue.Elem()
	}

	if eventValue.Kind() != reflect.Struct {
		return fmt.Errorf("expected struct, got %v", eventValue.Kind())
	}

	// Extract fields by name (safer than by index)
	var event, id string
	var retry int
	var data interface{}

	eventType := eventValue.Type()
	for i := 0; i < eventValue.NumField(); i++ {
		field := eventType.Field(i)
		fieldValue := eventValue.Field(i)

		switch field.Name {
		case "Event":
			if fieldValue.Kind() == reflect.String {
				event = fieldValue.String()
			}
		case "ID":
			if fieldValue.Kind() == reflect.String {
				id = fieldValue.String()
			}
		case "Retry":
			if fieldValue.Kind() == reflect.Int {
				retry = int(fieldValue.Int())
			}
		case "Data":
			data = fieldValue.Interface()
		}
	}

	// Write SSE fields
	if event != "" {
		if _, err := fmt.Fprintf(w, "event: %s\n", event); err != nil {
			return err
		}
	}
	if id != "" {
		if _, err := fmt.Fprintf(w, "id: %s\n", id); err != nil {
			return err
		}
	}
	if retry > 0 {
		if _, err := fmt.Fprintf(w, "retry: %d\n", retry); err != nil {
			return err
		}
	}

	// Marshal data as JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	// Write data field (handle multiline JSON)
	dataStr := string(jsonData)
	for _, line := range strings.Split(dataStr, "\n") {
		if _, err := fmt.Fprintf(w, "data: %s\n", line); err != nil {
			return err
		}
	}

	// Write empty line to end event
	if _, err := fmt.Fprintf(w, "\n"); err != nil {
		return err
	}

	return nil
}

// Helper functions - SIMPLIFIED
func isIterSeqOfEventData(t reflect.Type) bool {
	// iter.Seq[T] is func(func(T) bool)
	if t.Kind() != reflect.Func {
		return false
	}

	if t.NumIn() != 1 || t.NumOut() != 0 {
		return false
	}

	// Check the parameter type: func(EventData[T]) bool
	paramType := t.In(0)
	if paramType.Kind() != reflect.Func {
		return false
	}

	if paramType.NumIn() != 1 || paramType.NumOut() != 1 {
		return false
	}

	// Check return type is bool
	if paramType.Out(0).Kind() != reflect.Bool {
		return false
	}

	// Check if parameter looks like EventData[T]
	eventDataType := paramType.In(0)
	return looksLikeEventData(eventDataType)
}

func looksLikeEventData(t reflect.Type) bool {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	if t.Kind() != reflect.Struct {
		return false
	}

	// Check for EventData-like structure (has at least Event, Data, ID, Retry fields)
	hasData := false
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.Name == "Data" {
			hasData = true
			break
		}
	}

	return hasData
}

func extractEventDataType(iterType reflect.Type) reflect.Type {
	// Navigate: iter.Seq[EventData[T]] -> EventData[T] -> T (Data field type)
	if iterType.Kind() != reflect.Func {
		return nil
	}

	paramType := iterType.In(0) // func(EventData[T]) bool
	if paramType.Kind() != reflect.Func {
		return nil
	}

	eventDataType := paramType.In(0) // EventData[T]
	if eventDataType.Kind() == reflect.Ptr {
		eventDataType = eventDataType.Elem()
	}

	// Find the Data field and return its type
	for i := 0; i < eventDataType.NumField(); i++ {
		field := eventDataType.Field(i)
		if field.Name == "Data" {
			return field.Type
		}
	}

	return nil
}
