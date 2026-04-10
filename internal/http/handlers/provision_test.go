package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"simplek8/internal/apierror"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v4"
)

type testValidator struct {
	validator *validator.Validate
}

func (v *testValidator) Validate(i interface{}) error {
	return v.validator.Struct(i)
}

func setupTestEcho() *echo.Echo {
	e := echo.New()
	e.Validator = &testValidator{validator: validator.New()}
	return e
}

func TestHandleProvision_Validation(t *testing.T) {
	e := setupTestEcho()
	h := NewProvisionHandler(nil, nil)

	// Test case: Missing stackName
	reqBody := `{"region": "eastus"}`
	req := httptest.NewRequest(http.MethodPost, "/api/provision", bytes.NewBufferString(reqBody))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.HandleProvision(c)

	// In Echo, if a handler returns an error (apierror.Error implements error interface),
	// Echo's global error handler formats it. But apierror.Respond directly writes to response.
	// Since HandleProvision returns apierror.Respond(c, ...), it actually writes to the Response and returns nil.
	if err != nil {
		t.Fatalf("expected nil error (since apierror.Respond writes it), got %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}

	var apiErr apierror.Error
	if err := json.Unmarshal(rec.Body.Bytes(), &apiErr); err != nil {
		t.Fatalf("failed to parse response body: %v", err)
	}

	if apiErr.Code != apierror.InvalidRequestField {
		t.Errorf("expected error code %s, got %s", apierror.InvalidRequestField, apiErr.Code)
	}
}

func TestHandleDestroy_Validation(t *testing.T) {
	e := setupTestEcho()
	h := NewProvisionHandler(nil, nil)

	// Test case: StackName too short
	reqBody := `{"stackName": "ab"}`
	req := httptest.NewRequest(http.MethodPost, "/api/destroy", bytes.NewBufferString(reqBody))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	_ = h.HandleDestroy(c)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}

	var apiErr apierror.Error
	_ = json.Unmarshal(rec.Body.Bytes(), &apiErr)

	if apiErr.Message != "stackName must be at least 3 characters" {
		t.Errorf("expected message 'stackName must be at least 3 characters', got %q", apiErr.Message)
	}
}
