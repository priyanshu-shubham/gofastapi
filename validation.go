package gofastapi

import (
	"fmt"
	"sync"

	"github.com/go-playground/validator/v10"
)

var (
	validatorInstance *validator.Validate
	validatorOnce     sync.Once
)

// AddValidationRule adds a custom validation rule to the validator
func addValidationRule(tag string, fn validator.Func) error {
	v := getValidator()
	return v.RegisterValidation(tag, fn)
}

// getValidator returns a singleton validator instance
func getValidator() *validator.Validate {
	validatorOnce.Do(func() {
		validatorInstance = validator.New()
		validatorInstance.SetTagName("validate")
	})
	return validatorInstance
}

// validateStruct validates a struct using the validator tags
func validateStruct(obj interface{}, fieldValidators map[int]string) error {
	v := getValidator()

	// If we have field-level validators, we need to validate the entire struct
	if err := v.Struct(obj); err != nil {
		if validationErrors, ok := err.(validator.ValidationErrors); ok {
			fields := make(map[string][]string)
			for _, fieldErr := range validationErrors {
				fields[fieldErr.Field()] = append(fields[fieldErr.Field()],
					fmt.Sprintf("failed %s validation", fieldErr.Tag()))
			}
			return NewValidationError(fields)
		}
		return err
	}

	return nil
}
