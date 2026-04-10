package handlers

import (
	"strings"
	"testing"

	"github.com/go-playground/validator/v10"
)

func TestLowerFirst(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"StackName", "stackName"},
		{"Name", "name"},
		{"", ""},
		{"a", "a"},
		{"ABC", "aBC"},
	}

	for _, tt := range tests {
		got := lowerFirst(tt.input)
		if got != tt.want {
			t.Errorf("lowerFirst(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestValidationError_RequiredField(t *testing.T) {
	// Use a test struct with a required field to trigger a validation error.
	type testReq struct {
		StackName string `validate:"required"`
	}

	v := validator.New()
	err := v.Struct(&testReq{StackName: ""})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}

	apiErr := validationError(err)
	if apiErr.Status != 400 {
		t.Errorf("expected status 400, got %d", apiErr.Status)
	}
	if !strings.Contains(apiErr.Message, "stackName") {
		t.Errorf("expected message to contain 'stackName', got: %s", apiErr.Message)
	}
	if !strings.Contains(apiErr.Message, "required") {
		t.Errorf("expected message to contain 'required', got: %s", apiErr.Message)
	}
}

func TestValidationError_NonValidatorError(t *testing.T) {
	// Pass an error that is not ValidationErrors (e.g., from Struct(nil))
	apiErr := validationError(validator.New().Struct(nil))
	if apiErr == nil {
		t.Fatal("expected non-nil apierror")
	}
	if apiErr.Status != 400 {
		t.Errorf("expected status 400, got %d", apiErr.Status)
	}
}
