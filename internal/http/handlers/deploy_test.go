package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"simplek8/internal/apierror"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestPostDeploy_Validation(t *testing.T) {
	e := setupTestEcho() // Relies on setupTestEcho from provision_test.go
	h := NewDeployHandler(nil, nil) 

	// Test case: Invalid NodePort out of bounds
	reqBody := `{"nodePort": 20000}`
	req := httptest.NewRequest(http.MethodPost, "/api/deploy", bytes.NewBufferString(reqBody))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	_ = h.PostDeploy(c)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}

	var apiErr apierror.Error
	_ = json.Unmarshal(rec.Body.Bytes(), &apiErr)

	if apiErr.Code != apierror.InvalidRequestField {
		t.Errorf("expected error code %s, got %s", apierror.InvalidRequestField, apiErr.Code)
	}
	if apiErr.Message != "nodePort must be at least 30000 characters" {
		t.Errorf("unexpected message: %q", apiErr.Message)
	}
}
