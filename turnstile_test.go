package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
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

func TestWebSocket_TurnstileRejection(t *testing.T) {
	os.Setenv("TURNSTILE_SECRET_KEY", "test_secret_key_123")
	defer os.Unsetenv("TURNSTILE_SECRET_KEY")

	server := httptest.NewServer(http.HandlerFunc(handleWebSocket))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	c, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial websocket: %v", err)
	}
	defer c.Close(websocket.StatusNormalClosure, "")

	// Send message without turnstile_token
	inMsg := WSIncomingMessage{
		Action:         "start",
		Image:          "data:image/jpeg;base64,/9j/4AAQSkZJRg==",
		TurnstileToken: "",
	}
	if err := wsjson.Write(ctx, c, inMsg); err != nil {
		t.Fatalf("failed to write message: %v", err)
	}

	var outMsg WSOutgoingMessage
	if err := wsjson.Read(ctx, c, &outMsg); err != nil {
		t.Fatalf("failed to read response: %v", err)
	}

	if outMsg.Type != "error" || !strings.Contains(outMsg.Message, "Turnstile") {
		t.Fatalf("expected error message mentioning Turnstile, got: type=%s, message=%s", outMsg.Type, outMsg.Message)
	}
}

func TestGetClientIP(t *testing.T) {
	// CF-Connecting-IP takes highest precedence
	req1 := httptest.NewRequest("GET", "/", nil)
	req1.Header.Set("CF-Connecting-IP", "103.21.244.2")
	req1.Header.Set("X-Forwarded-For", "198.51.100.1, 198.51.100.2")
	if ip := getClientIP(req1); ip != "103.21.244.2" {
		t.Errorf("expected CF-Connecting-IP, got %s", ip)
	}

	// Comma-separated X-Forwarded-For extracts first client IP
	req2 := httptest.NewRequest("GET", "/", nil)
	req2.Header.Set("X-Forwarded-For", "203.0.113.195, 70.41.3.18, 150.172.238.178")
	if ip := getClientIP(req2); ip != "203.0.113.195" {
		t.Errorf("expected first X-Forwarded-For IP, got %s", ip)
	}

	// IPv6 RemoteAddr safely parsed
	req3 := httptest.NewRequest("GET", "/", nil)
	req3.RemoteAddr = "[2001:db8::1]:54321"
	if ip := getClientIP(req3); ip != "2001:db8::1" {
		t.Errorf("expected IPv6 host, got %s", ip)
	}
}
