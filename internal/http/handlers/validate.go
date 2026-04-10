package handlers

import (
	"fmt"
	"strings"

	"github.com/go-playground/validator/v10"
	"simplek8/internal/apierror"
)

// validationError converts a validator.ValidationErrors into a structured apierror.
// It formats the first failing field into a human-readable message.
//
// Example output: &apierror.Error{Code: "INVALID_FIELD", Message: "stackName is required", Status: 400}
func validationError(err error) *apierror.Error {
	validationErrors, ok := err.(validator.ValidationErrors)
	if !ok {
		return apierror.BadRequest(apierror.InvalidRequestBody, "invalid request payload")
	}

	if len(validationErrors) == 0 {
		return apierror.BadRequest(apierror.InvalidRequestBody, "validation failed")
	}

	// Report only the first validation error to keep messages simple.
	fe := validationErrors[0]

	// Convert the struct field name (e.g. "StackName") to its JSON key (e.g. "stackName")
	// by lowercasing the first character. This is a simple heuristic that works for
	// the JSON tags used in this project.
	field := lowerFirst(fe.Field())

	var msg string
	switch fe.Tag() {
	case "required":
		msg = fmt.Sprintf("%s is required", field)
	case "min":
		msg = fmt.Sprintf("%s must be at least %s characters", field, fe.Param())
	case "max":
		msg = fmt.Sprintf("%s must be at most %s characters", field, fe.Param())
	case "oneof":
		msg = fmt.Sprintf("%s must be one of: %s", field, fe.Param())
	case "url", "http_url":
		msg = fmt.Sprintf("%s must be a valid URL", field)
	case "uuid4", "uuid":
		msg = fmt.Sprintf("%s must be a valid UUID", field)
	default:
		msg = fmt.Sprintf("%s failed validation: %s", field, fe.Tag())
	}

	return apierror.BadRequest(apierror.InvalidRequestField, msg)
}

// lowerFirst returns s with its first character lowercased.
// "StackName" -> "stackName", "Name" -> "name", "" -> ""
func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}
