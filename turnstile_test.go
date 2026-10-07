package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVerifyTurnstileToken_BypassWhenNoSecret(t *testing.T) {
	ok, err := verifyTurnstileToken("", "some-token", "127.0.0.1")
	if err != nil {
		t.Fatalf("expected no error on bypass, got %v", err)
	}
	if !ok {
		t.Fatalf("expected ok=true when secret is empty")
	}
}

func TestVerifyTurnstileToken_FailsOnEmptyTokenWhenSecretSet(t *testing.T) {
	ok, err := verifyTurnstileToken("0x4AAAAAA_secret", "", "127.0.0.1")
	if err == nil {
		t.Fatalf("expected error on empty token when secret is set")
	}
	if ok {
		t.Fatalf("expected ok=false on empty token")
	}
}

func TestConfigEndpoint(t *testing.T) {
	req := httptest.NewRequest("GET", "/config", nil)
	rec := httptest.NewRecorder()

	mux := http.NewServeMux()
	registerConfigEndpoint(mux)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var res map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if _, exists := res["turnstile_enabled"]; !exists {
		t.Fatalf("expected turnstile_enabled key in response")
	}
}
