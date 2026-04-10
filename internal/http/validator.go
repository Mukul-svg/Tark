package http

import (
	"github.com/go-playground/validator/v10"
)

// AppValidator wraps go-playground/validator for use with Echo's c.Validate().
type AppValidator struct {
	validator *validator.Validate
}

// NewAppValidator creates an AppValidator with a default validator instance.
func NewAppValidator() *AppValidator {
	return &AppValidator{validator: validator.New()}
}

// Validate runs struct-level validation using the `validate` struct tags.
// Returns nil if validation passes, or a validator.ValidationErrors if it fails.
func (v *AppValidator) Validate(i interface{}) error {
	return v.validator.Struct(i)
}
