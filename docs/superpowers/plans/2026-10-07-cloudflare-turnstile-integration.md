# Cloudflare Turnstile Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Mengintegrasikan Cloudflare Turnstile anti-bot protection ke dalam WhatsApp Full Profile Picture (`ppfull`) dengan backend Go di Hugging Face Spaces dan frontend vanilla JS di Vercel.

**Architecture:** Backend menyediakan endpoint `GET /config` untuk mendistribusikan `turnstile_site_key` secara dinamis. Frontend merender widget Turnstile (Managed Mode) di atas tombol aksi, kemudian menyertakan `turnstile_token` dalam pesan inisialisasi WebSocket `{ action: "start" }`. Backend memvalidasi token ke Cloudflare Siteverify API sebelum mengalokasikan sesi WhatsApp whatsmeow.

**Tech Stack:** Go (Standard Library `net/http`, `encoding/json`), Cloudflare Turnstile REST API, Vanilla JavaScript, HTML5, CSS3, WebSocket (`github.com/coder/websocket`).

**Spec:** [docs/superpowers/specs/2026-10-07-cloudflare-turnstile-integration-design.md](file:///c:/Users/Administrator/Documents/PROJECT-GITHUB/ppfull/docs/superpowers/specs/2026-10-07-cloudflare-turnstile-integration-design.md)

## Global Constraints

- Backend server port default `7860` (Hugging Face Spaces) atau membaca variabel `PORT`.
- CORS aktif untuk semua origin `*` pada endpoint HTTP `/health`, `/config`, dan WebSocket `/ws`.
- Dev/Bypass Mode: Jika `TURNSTILE_SECRET_KEY` tidak diatur di environment, backend meloloskan verifikasi dengan peringatan log (tidak memblokir pengujian lokal).
- Single-use token: Token Turnstile harus segera direset di frontend setelah dikirimkan agar tidak terjadi replay attack.

## Review Focus

1. Client mengirim `turnstile_token` kosong saat backend mengaktifkan `TURNSTILE_SECRET_KEY` -> Backend menolak dengan error deskriptif dan menutup websocket.
2. Cloudflare API mengembalikan HTTP error atau network timeout -> Backend mengembalikan error ramah pengguna tanpa panic atau crash.
3. Hugging Face backend dalam keadaan sleeping / waking up -> Frontend tidak crash saat memuat widget dan menunggu backend online sebelum merender widget.
4. Token Turnstile kedaluwarsa sebelum user klik "Mulai Hubungkan" -> Callback `expired-callback` mereset token dan tombol kembali terkunci hingga diverifikasi ulang.
5. Mode dev / pengujian lokal tanpa internet ke Cloudflare -> Backend tetap dapat berjalan mulus tanpa membutuhkan API key Cloudflare.

---

### Task 1: Backend Turnstile Verification Logic & `/config` Endpoint

**Files:**
- Create: `turnstile_test.go`
- Modify: `main.go:38-75`

**Interfaces:**
- Produces:
  - `type TurnstileVerifyResponse struct`
  - `func verifyTurnstileToken(secretKey, token, remoteIP string) (bool, error)`
  - `GET /config` handler returning JSON `{"turnstile_enabled": bool, "turnstile_site_key": string}`
  - Updated `WSIncomingMessage` struct with `TurnstileToken string`

- [ ] **Step 1: Write failing unit tests for Turnstile verification logic**

Create `turnstile_test.go`:
```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v -run "TestVerifyTurnstileToken|TestConfigEndpoint"`
Expected: FAIL (undefined `verifyTurnstileToken`, undefined `registerConfigEndpoint`).

- [ ] **Step 3: Implement `verifyTurnstileToken` and `/config` endpoint in `main.go`**

- Add `TurnstileToken` field to `WSIncomingMessage`.
- Implement `TurnstileVerifyResponse` struct.
- Implement `verifyTurnstileToken(secretKey, token, remoteIP string) (bool, error)` making a POST request with `url.Values` to `https://challenges.cloudflare.com/turnstile/v0/siteverify` with a 10s timeout client.
- Implement `registerConfigEndpoint(mux *http.ServeMux)` returning `{ "turnstile_enabled": isEnabled, "turnstile_site_key": siteKey }` with CORS headers enabled.
- Register `registerConfigEndpoint(mux)` in `main()`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -v -run "TestVerifyTurnstileToken|TestConfigEndpoint"`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add main.go turnstile_test.go
git commit -m "feat(backend): add turnstile verification logic and /config endpoint"
```

---

### Task 2: Backend WebSocket Turnstile Validation Enforcement

**Files:**
- Modify: `main.go:130-170`
- Modify: `turnstile_test.go`

**Interfaces:**
- Consumes: `verifyTurnstileToken()` from Task 1, `WSIncomingMessage.TurnstileToken`
- Produces: Enforcement in `handleWebSocket` before processing image or initializing SQLite/Whatsmeow session.

- [ ] **Step 1: Write integration test for WebSocket Turnstile token rejection**

Add `TestWebSocket_TurnstileRejection` to `turnstile_test.go` testing that a client without a token or with an invalid token gets an error message when secret key is set.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v -run TestWebSocket_TurnstileRejection`
Expected: FAIL.

- [ ] **Step 3: Implement validation check in `handleWebSocket` in `main.go`**

In `handleWebSocket`, right after reading `inMsg`:
```go
secretKey := os.Getenv("TURNSTILE_SECRET_KEY")
if secretKey != "" {
    clientIP := r.Header.Get("CF-Connecting-IP")
    if clientIP == "" {
        clientIP = r.Header.Get("X-Forwarded-For")
    }
    if clientIP == "" {
        clientIP = strings.Split(r.RemoteAddr, ":")[0]
    }

    ok, err := verifyTurnstileToken(secretKey, inMsg.TurnstileToken, clientIP)
    if !ok || err != nil {
        log.Printf("[WS] Turnstile verification failed for %s: %v", r.RemoteAddr, err)
        _ = wsjson.Write(ctx, c, WSOutgoingMessage{
            Type:    "error",
            Message: "Verifikasi keamanan Turnstile gagal atau kedaluwarsa. Silakan muat ulang halaman.",
        })
        return
    }
    log.Printf("[WS] Turnstile verification succeeded for %s", r.RemoteAddr)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -v ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add main.go turnstile_test.go
git commit -m "feat(backend): enforce turnstile verification on websocket initiation"
```

---

### Task 3: Frontend Turnstile Widget, CSS & Dynamic Config Integration

**Files:**
- Modify: `frontend/index.html:14-16,160-175`
- Modify: `frontend/style.css`
- Modify: `frontend/app.js:25-50,110-150,230-245,300-360`

**Interfaces:**
- Consumes: `GET /config` endpoint from Task 1, Cloudflare Turnstile API JS (`window.turnstile`)
- Produces: Turnstile widget rendering, token state tracking, token payload transmission in `startProcess()`.

- [ ] **Step 1: Update `frontend/index.html`**

- Add Cloudflare Turnstile script tag to `<head>`:
  `<script src="https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit" async defer></script>`
- Add `#turnstile-container-wrapper` inside `#view-upload` above `#btn-start-process`:
  ```html
  <div id="turnstile-wrapper" class="turnstile-wrapper hidden">
    <div id="turnstile-widget"></div>
    <div id="turnstile-feedback" class="turnstile-feedback"></div>
  </div>
  ```

- [ ] **Step 2: Add Turnstile styles in `frontend/style.css`**

- Add styles for `.turnstile-wrapper` (centered, glassmorphism border, margin, flex layout, dark-theme alignment).
- Add styles for `.turnstile-feedback` (subtle helper text, error indicator).

- [ ] **Step 3: Implement config fetching & Turnstile lifecycle in `frontend/app.js`**

- In `state`: add `turnstileEnabled: false`, `turnstileSiteKey: null`, `turnstileToken: null`, `turnstileWidgetId: null`.
- Update `updateStartButtonState()`: button is enabled if `state.currentImageBase64` exists AND (`!state.turnstileEnabled` OR `state.turnstileToken` is not null).
- In `fetchBackendConfig()`: fetch `${base}/config`, read `turnstile_enabled` and `turnstile_site_key`.
- If `turnstile_enabled` is true:
  - Unhide `#turnstile-wrapper`.
  - Render widget via `turnstile.render('#turnstile-widget', { sitekey, theme: 'dark', callback: (token) => { state.turnstileToken = token; updateStartButtonState(); }, 'expired-callback': () => { state.turnstileToken = null; updateStartButtonState(); turnstile.reset(state.turnstileWidgetId); }, 'error-callback': () => { ... } })`.
- In `startProcess()`:
  - Add `turnstile_token: state.turnstileToken` to payload sent via WebSocket `onopen`.
  - Reset turnstile token and widget on cancel or error or completion so token cannot be reused.

- [ ] **Step 4: Manual/Browser validation & verification**

Verify syntax and structure of `frontend/app.js`, `frontend/index.html`, and `frontend/style.css`.
Run local server test to verify `/config` endpoint responds and static files are served cleanly.

- [ ] **Step 5: Commit**

```bash
git add frontend/index.html frontend/style.css frontend/app.js
git commit -m "feat(frontend): integrate cloudflare turnstile widget and dynamic config"
```

---

### Task 4: Environment Configuration & Documentation

**Files:**
- Modify: `.env.example`
- Modify: `README.md`

- [ ] **Step 1: Update `.env.example`**

Add configuration variables:
```env
PAIRING_NUMBER="628xxxxxxxx"
PFP_PATH="pfp.jpg"
PORT="7860"
TURNSTILE_SITE_KEY=""
TURNSTILE_SECRET_KEY=""
```

- [ ] **Step 2: Update `README.md`**

Add a dedicated section explaining:
- How to get Cloudflare Turnstile keys (Free on Cloudflare Dashboard).
- How to set `TURNSTILE_SITE_KEY` and `TURNSTILE_SECRET_KEY` on Hugging Face Spaces (Repository Secrets/Environment Variables).
- How the Vercel frontend automatically detects and renders the widget without needing redeployment.

- [ ] **Step 3: Run full verification build**

Run: `go build -v .` and `go test -v ./...`
Expected: Successful compile and all tests passing.

- [ ] **Step 4: Commit**

```bash
git add .env.example README.md
git commit -m "docs: add cloudflare turnstile configuration guidelines"
```
