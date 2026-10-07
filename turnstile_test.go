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
