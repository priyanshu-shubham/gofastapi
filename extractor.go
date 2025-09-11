package gofastapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
)

// FieldExtractor extracts a field value from an HTTP request
type FieldExtractor interface {
	Extract(r *http.Request, vars map[string]string, body []byte) (interface{}, error)
}

// PathExtractor extracts path parameters
type PathExtractor struct {
	paramName string
	fieldType reflect.Type
}

func (e *PathExtractor) Extract(r *http.Request, vars map[string]string, body []byte) (interface{}, error) {
	value, ok := vars[e.paramName]
	if !ok {
		return nil, fmt.Errorf("path parameter %s not found", e.paramName)
	}
	return convertValue(value, e.fieldType)
}

// QueryExtractor extracts query parameters
type QueryExtractor struct {
	paramName string
	fieldType reflect.Type
}

func (e *QueryExtractor) Extract(r *http.Request, vars map[string]string, body []byte) (interface{}, error) {
	value := r.URL.Query().Get(e.paramName)
	if value == "" && e.fieldType.Kind() != reflect.Bool {
		return reflect.Zero(e.fieldType).Interface(), nil
	}
	return convertValue(value, e.fieldType)
}

// HeaderExtractor extracts headers
type HeaderExtractor struct {
	headerName string
	fieldType  reflect.Type
}

func (e *HeaderExtractor) Extract(r *http.Request, vars map[string]string, body []byte) (interface{}, error) {
	value := r.Header.Get(e.headerName)
	if value == "" {
		return reflect.Zero(e.fieldType).Interface(), nil
	}
	return convertValue(value, e.fieldType)
}

// JSONExtractor extracts fields from JSON body
type JSONExtractor struct {
	jsonPath  string
	fieldType reflect.Type
}

func (e *JSONExtractor) Extract(r *http.Request, vars map[string]string, body []byte) (interface{}, error) {
	if len(body) == 0 {
		return reflect.Zero(e.fieldType).Interface(), nil
	}

	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		// Try unmarshaling into the field directly if it's the entire body
		result := reflect.New(e.fieldType).Interface()
		if err := json.Unmarshal(body, result); err != nil {
			return nil, err
		}
		return reflect.ValueOf(result).Elem().Interface(), nil
	}

	value, ok := data[e.jsonPath]
	if !ok {
		return reflect.Zero(e.fieldType).Interface(), nil
	}

	// Re-marshal and unmarshal to handle complex types
	jsonBytes, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}

	result := reflect.New(e.fieldType).Interface()
	if err := json.Unmarshal(jsonBytes, result); err != nil {
		return nil, err
	}

	return reflect.ValueOf(result).Elem().Interface(), nil
}

// DependencyExtractor extracts values from resolved dependencies
type DependencyExtractor struct {
	depName   string
	fieldPath []string // For nested access like "auth.user_id"
	fieldType reflect.Type
}

func (e *DependencyExtractor) Extract(r *http.Request, vars map[string]string, body []byte) (interface{}, error) {
	// This will be populated by the dependency resolver
	return nil, nil
}

// convertValue converts string values to the target type
func convertValue(value string, targetType reflect.Type) (interface{}, error) {
	switch targetType.Kind() {
	case reflect.String:
		return value, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.ParseInt(value, 10, targetType.Bits())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.ParseUint(value, 10, targetType.Bits())
	case reflect.Float32, reflect.Float64:
		return strconv.ParseFloat(value, targetType.Bits())
	case reflect.Bool:
		return strconv.ParseBool(value)
	case reflect.Slice:
		// Handle comma-separated values for slices
		parts := strings.Split(value, ",")
		slice := reflect.MakeSlice(targetType, len(parts), len(parts))
		elemType := targetType.Elem()
		for i, part := range parts {
			elem, err := convertValue(strings.TrimSpace(part), elemType)
			if err != nil {
				return nil, err
			}
			slice.Index(i).Set(reflect.ValueOf(elem))
		}
		return slice.Interface(), nil
	default:
		return nil, fmt.Errorf("unsupported type: %v", targetType)
	}
}
