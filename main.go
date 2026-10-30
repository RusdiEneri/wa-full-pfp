package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/disintegration/imaging"
	_ "github.com/glebarez/go-sqlite"
	"github.com/google/uuid"
	"github.com/subosito/gotenv"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCompanionReg"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"rsc.io/qr"
)

//go:embed all:frontend
var embeddedFrontend embed.FS

// WSIncomingMessage represents requests sent by the frontend
type WSIncomingMessage struct {
	Action         string `json:"action"`                    // "start"
	Image          string `json:"image"`                     // Base64 data URL
	PairNumber     string `json:"pair_number"`               // Optional phone number for pairing code
	TurnstileToken string `json:"turnstile_token,omitempty"` // Cloudflare Turnstile token
}

// WSOutgoingMessage represents messages sent to the frontend
type WSOutgoingMessage struct {
	Type    string `json:"type"`               // "status", "qr", "pairing_code", "success", "error"
	Message string `json:"message,omitempty"`  // Status description
	Code    string `json:"code,omitempty"`     // Raw QR or Pairing code
	QRImage string `json:"qr_image,omitempty"` // Base64 PNG data URL of QR code
}

// TurnstileVerifyResponse represents the response from Cloudflare Siteverify API
type TurnstileVerifyResponse struct {
	Success     bool      `json:"success"`
	ChallengeTS time.Time `json:"challenge_ts"`
	Hostname    string    `json:"hostname"`
	ErrorCodes  []string  `json:"error-codes"`
	Action      string    `json:"action"`
	Cdata       string    `json:"cdata"`
}

// verifyTurnstileToken verifies a Cloudflare Turnstile token
func verifyTurnstileToken(secretKey, token, remoteIP string) (bool, error) {
	if secretKey == "" {
		log.Println("[Turnstile] Secret key tidak dikonfigurasi. Mode bypass aktif untuk dev.")
		return true, nil
	}

	if strings.TrimSpace(token) == "" {
		return false, fmt.Errorf("token turnstile tidak boleh kosong")
	}

	data := url.Values{}
	data.Set("secret", secretKey)
	data.Set("response", token)
	if remoteIP != "" {
		data.Set("remoteip", remoteIP)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.PostForm("https://challenges.cloudflare.com/turnstile/v0/siteverify", data)
	if err != nil {
		return false, fmt.Errorf("gagal menghubungi api turnstile: %w", err)
	}
	defer resp.Body.Close()

	var result TurnstileVerifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, fmt.Errorf("gagal decode response turnstile: %w", err)
	}

	if !result.Success {
		return false, fmt.Errorf("verifikasi turnstile ditolak: %v", result.ErrorCodes)
	}

	return true, nil
}

func getClientIP(r *http.Request) string {
	if cfIP := r.Header.Get("CF-Connecting-IP"); cfIP != "" {
		return strings.TrimSpace(cfIP)
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

// registerConfigEndpoint exposes public configuration (e.g. Turnstile site key) to the frontend
func registerConfigEndpoint(mux *http.ServeMux) {
	mux.HandleFunc("/config", func(w http.ResponseWriter, r *http.Request) {
		enableCORS(w)
		w.Header().Set("Content-Type", "application/json")

		siteKey := os.Getenv("TURNSTILE_SITE_KEY")
		isEnabled := siteKey != ""

		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"turnstile_enabled":  isEnabled,
			"turnstile_site_key": siteKey,
		})
	})
}

func main() {
	_ = gotenv.Load()

	port := os.Getenv("PORT")
	if port == "" {
		port = "7860" // Default Hugging Face Spaces port
	}

	mux := http.NewServeMux()

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		enableCORS(w)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "healthy",
			"service": "WhatsApp Full Profile Picture Server",
			"time":    time.Now().Format(time.RFC3339),
		})
	})

	// Public config endpoint
	registerConfigEndpoint(mux)

	// WebSocket handler for WhatsApp session
	mux.HandleFunc("/ws", handleWebSocket)

	// Static frontend handler (embedded)
	frontendSub, err := fs.Sub(embeddedFrontend, "frontend")
	if err == nil {
		fileServer := http.FileServer(http.FS(frontendSub))
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			cleanPath := strings.TrimPrefix(filepath.Clean(r.URL.Path), string(filepath.Separator))
			cleanPath = strings.ReplaceAll(cleanPath, "\\", "/")
			if cleanPath == "" || cleanPath == "." {
				cleanPath = "index.html"
			}
			if _, statErr := fs.Stat(frontendSub, cleanPath); os.IsNotExist(statErr) {
				w.WriteHeader(http.StatusNotFound)
				if notFoundData, readErr := fs.ReadFile(frontendSub, "404.html"); readErr == nil {
					w.Header().Set("Content-Type", "text/html; charset=utf-8")
					_, _ = w.Write(notFoundData)
					return
				}
			}
			fileServer.ServeHTTP(w, r)
		})
	} else {
		log.Printf("Warning: Failed to load embedded frontend: %v. Serving disk frontend/ if exists.", err)
		mux.Handle("/", http.FileServer(http.Dir("./frontend")))
	}

	// Wrapper with CORS and logging
	handler := enableCORSHandler(mux)

	log.Printf("==================================================")
	log.Printf("  WA Full Profile Picture Web Server Running")
	log.Printf("  Listening on http://0.0.0.0:%s", port)
	log.Printf("  Hugging Face / Vercel Ready")
	log.Printf("==================================================")

	if err := http.ListenAndServe(":"+port, handler); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func enableCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
}

func enableCORSHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		enableCORS(w)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Accept WebSocket connection with cross-origin support
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		log.Printf("[WS] Accept error: %v", err)
		return
	}
	defer c.Close(websocket.StatusInternalError, "session ended")

	// Set read limit to 50MB (default is 32KB which drops large image uploads!)
	c.SetReadLimit(50 * 1024 * 1024)
	log.Printf("[WS] Client connected from %s", r.RemoteAddr)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Read initial configuration & image from client
	var inMsg WSIncomingMessage
	if err := wsjson.Read(ctx, c, &inMsg); err != nil {
		log.Printf("[WS] Failed to read init message from %s: %v", r.RemoteAddr, err)
		return
	}

	log.Printf("[WS] Init message received: action=%s, image_bytes=%d, pair_number=%s",
		inMsg.Action, len(inMsg.Image), inMsg.PairNumber)

	if inMsg.Action != "start" || inMsg.Image == "" {
		_ = wsjson.Write(ctx, c, WSOutgoingMessage{
			Type:    "error",
			Message: "Format data tidak valid. Silakan unggah gambar terlebih dahulu.",
		})
		return
	}

	// Validasi keamanan Cloudflare Turnstile jika diaktifkan di server
	secretKey := os.Getenv("TURNSTILE_SECRET_KEY")
	if secretKey != "" {
		clientIP := getClientIP(r)
		ok, err := verifyTurnstileToken(secretKey, inMsg.TurnstileToken, clientIP)
		if !ok || err != nil {
			log.Printf("[WS] Turnstile verification failed for %s: %v", r.RemoteAddr, err)
			_ = wsjson.Write(ctx, c, WSOutgoingMessage{
				Type:    "error",
				Message: "Verifikasi keamanan Turnstile gagal atau kedaluwarsa. Silakan muat ulang halaman dan coba lagi.",
			})
			return
		}
		log.Printf("[WS] Turnstile verification succeeded for %s", r.RemoteAddr)
	}

	_ = wsjson.Write(ctx, c, WSOutgoingMessage{
		Type:    "status",
		Message: "Memproses gambar profil...",
	})

	// Process image
	processedImg, err := processImage(inMsg.Image)
	if err != nil {
		_ = wsjson.Write(ctx, c, WSOutgoingMessage{
			Type:    "error",
			Message: fmt.Sprintf("Gagal memproses gambar: %v", err),
		})
		return
	}

	// Generate isolated temporary SQLite session
	sessionID := uuid.New().String()
	dbDir := filepath.Join(os.TempDir(), "wa_pfp_sessions")
	_ = os.MkdirAll(dbDir, 0700)
	dbFile := filepath.Join(dbDir, fmt.Sprintf("sess_%s.db", sessionID))
	dbURI := fmt.Sprintf("file:%s?_pragma=foreign_keys=on", filepath.ToSlash(dbFile))

	dbLog := waLog.Stdout("Database", "ERROR", true)
	container, err := sqlstore.New(ctx, "sqlite", dbURI, dbLog)
	if err != nil {
		_ = wsjson.Write(ctx, c, WSOutgoingMessage{
			Type:    "error",
			Message: "Gagal menginisialisasi database sesi sementara.",
		})
		return
	}

	// Session cleanup on disconnect or exit
	var client *whatsmeow.Client
	var loggedOut bool
	defer func() {
		log.Printf("[Session %s] Membersihkan sesi...", sessionID)
		if !loggedOut && client != nil && client.Store != nil && client.Store.ID != nil {
			log.Printf("[Session %s] Fallback cleanup: memastikan sesi di-logout...", sessionID)
			if err := logoutSession(client, sessionID); err != nil {
				log.Printf("[Session %s] Fallback cleanup gagal: %v", sessionID, err)
			}
		}
		if client != nil && client.IsConnected() {
			client.Disconnect()
		}
		_ = container.Close()
		_ = os.Remove(dbFile)
		_ = os.Remove(dbFile + "-wal")
		_ = os.Remove(dbFile + "-shm")
		log.Printf("[Session %s] Sesi selesai dan dibersihkan.", sessionID)
	}()

	// Device properties (mimic Chrome on Windows sesuai spesifikasi dan docs whatsmeow)
	store.DeviceProps.PlatformType = waCompanionReg.DeviceProps_CHROME.Enum()
	store.SetOSInfo("Windows", [3]uint32{10, 0, 22631})

	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		_ = wsjson.Write(ctx, c, WSOutgoingMessage{
			Type:    "error",
			Message: "Gagal membuat device store.",
		})
		return
	}

	clientLog := waLog.Stdout("Client", "INFO", true)
	client = whatsmeow.NewClient(deviceStore, clientLog)

	connectedChan := make(chan struct{}, 5)
	loggedOutChan := make(chan struct{}, 5)

	client.AddEventHandler(func(rawEvt any) {
		switch evt := rawEvt.(type) {
		case *events.Connected:
			log.Printf("[Session %s] Event whatsmeow: Connected (terotentikasi)", sessionID)
			select {
			case connectedChan <- struct{}{}:
			default:
			}
		case *events.LoggedOut:
			log.Printf("[Session %s] Event whatsmeow: LoggedOut (reason: %v)", sessionID, evt.Reason)
			select {
			case loggedOutChan <- struct{}{}:
			default:
			}
		case *events.StreamReplaced:
			log.Printf("[Session %s] Event whatsmeow: StreamReplaced", sessionID)
			select {
			case loggedOutChan <- struct{}{}:
			default:
			}
		}
	})

	// Mode 1: Pairing code (via phone number)
	if inMsg.PairNumber != "" {
		pairNumber := regexp.MustCompile(`\D+`).ReplaceAllString(inMsg.PairNumber, "")

		// Validasi panjang nomor untuk memastikan ada kode negara
		if len(pairNumber) < 10 {
			_ = wsjson.Write(ctx, c, WSOutgoingMessage{
				Type:    "error",
				Message: "Format nomor telepon tidak valid. Pastikan menggunakan kode negara (contoh: 62812... untuk Indonesia, jangan gunakan 0812...).",
			})
			return
		}

		_ = wsjson.Write(ctx, c, WSOutgoingMessage{
			Type:    "status",
			Message: fmt.Sprintf("Menghubungkan kode pairing untuk nomor: %s...", pairNumber),
		})

		// Sesuai dokumentasi resmi whatsmeow (tulir):
		// GetQRChannel() harus dipanggil SEBELUM Connect().
		// Kita perlu menunggu item pertama dari channel untuk memastikan koneksi ke server WhatsApp
		// sudah siap penuh sebelum memanggil PairPhone.
		qrChan, err := client.GetQRChannel(ctx)
		if err != nil {
			_ = wsjson.Write(ctx, c, WSOutgoingMessage{
				Type:    "error",
				Message: fmt.Sprintf("Gagal inisialisasi pairing channel: %v", err),
			})
			return
		}

		if err := client.Connect(); err != nil {
			_ = wsjson.Write(ctx, c, WSOutgoingMessage{
				Type:    "error",
				Message: fmt.Sprintf("Koneksi gagal: %v", err),
			})
			return
		}

		// Tunggu item pertama dari qrChan agar koneksi siap penuh
		select {
		case item, ok := <-qrChan:
			if !ok {
				_ = wsjson.Write(ctx, c, WSOutgoingMessage{
					Type:    "error",
					Message: "Koneksi pairing terputus sebelum siap.",
				})
				return
			}
			if item.Event == "error" || item.Event == "err-client-outdated" {
				_ = wsjson.Write(ctx, c, WSOutgoingMessage{
					Type:    "error",
					Message: fmt.Sprintf("Server WhatsApp menolak koneksi (%s): %v", item.Event, item.Error),
				})
				return
			}
		case <-time.After(3 * time.Second):
			// Fallback timeout jika event QR tertunda
		case <-ctx.Done():
			return
		}

		// Menurut docs resmi whatsmeow:
		// Format clientDisplayName HARUS `Browser (OS)`, contoh: "Chrome (Windows)".
		// WhatsApp server memvalidasi string ini dan akan me-reject dengan error 400 jika tidak sesuai format.
		pairCtx, pairCancel := context.WithTimeout(context.Background(), 30*time.Second)
		code, err := client.PairPhone(pairCtx, pairNumber, true, whatsmeow.PairClientChrome, "Chrome (Windows)")
		pairCancel()

		if err != nil {
			errMsg := err.Error()
			if errors.Is(err, whatsmeow.ErrPhoneNumberIsNotInternational) {
				errMsg = "Nomor telepon harus menyertakan kode negara internasional (contoh: 62812..., bukan 0812...)."
			} else if errors.Is(err, whatsmeow.ErrPhoneNumberTooShort) {
				errMsg = "Nomor telepon terlalu pendek."
			} else if strings.Contains(errMsg, "400") || strings.Contains(errMsg, "bad-request") {
				errMsg = fmt.Sprintf("Permintaan ditolak oleh server WhatsApp (400 Bad Request). Kemungkinan: 1) Nomor tidak terdaftar di WhatsApp, 2) IP server diblokir WhatsApp (Cloud/Datacenter), atau 3) Terlalu banyak percobaan pairing. Detail: %v", err)
			}
			_ = wsjson.Write(ctx, c, WSOutgoingMessage{
				Type:    "error",
				Message: fmt.Sprintf("Gagal meminta kode pairing: %v", errMsg),
			})
			return
		}

		_ = wsjson.Write(ctx, c, WSOutgoingMessage{
			Type:    "pairing_code",
			Code:    code,
			Message: "Masukkan kode ini pada WhatsApp di HP Anda (Perangkat Tertaut -> Tautkan dengan nomor telepon)",
		})

		// Wait for login or timeout (160 seconds sesuai masa aktif websocket pairing WhatsApp)
		loginSuccess := false
		loginTimer := time.NewTimer(160 * time.Second)
		defer loginTimer.Stop()

	loginLoop:
		for !loginSuccess {
			if client.Store.ID != nil {
				loginSuccess = true
				break
			}
			select {
			case <-ctx.Done():
				return
			case <-loginTimer.C:
				_ = wsjson.Write(ctx, c, WSOutgoingMessage{
					Type:    "error",
					Message: "Waktu pairing habis (timeout). Silakan coba lagi.",
				})
				return
			case item, ok := <-qrChan:
				if !ok {
					if client.Store.ID != nil {
						loginSuccess = true
					}
					break loginLoop
				}
				if item.Event == "success" || client.Store.ID != nil {
					loginSuccess = true
					break loginLoop
				} else if item.Event == "error" {
					_ = wsjson.Write(ctx, c, WSOutgoingMessage{
						Type:    "error",
						Message: fmt.Sprintf("Pairing gagal: %v", item.Error),
					})
					return
				}
				// Event "code" dapat diabaikan saat pairing dengan nomor HP
			case <-connectedChan:
				loginSuccess = true
				break loginLoop
			case <-time.After(1 * time.Second):
				if client.Store.ID != nil {
					loginSuccess = true
					break loginLoop
				}
			}
		}

		if !loginSuccess && client.Store.ID == nil {
			_ = wsjson.Write(ctx, c, WSOutgoingMessage{
				Type:    "error",
				Message: "Waktu pairing habis (timeout). Silakan coba lagi.",
			})
			return
		}

	} else {
		// Mode 2: Scan QR Code
		qrChan, err := client.GetQRChannel(ctx)
		if err != nil {
			_ = wsjson.Write(ctx, c, WSOutgoingMessage{
				Type:    "error",
				Message: fmt.Sprintf("Gagal inisialisasi QR channel: %v", err),
			})
			return
		}

		if err := client.Connect(); err != nil {
			_ = wsjson.Write(ctx, c, WSOutgoingMessage{
				Type:    "error",
				Message: fmt.Sprintf("Koneksi WhatsApp gagal: %v", err),
			})
			return
		}

		_ = wsjson.Write(ctx, c, WSOutgoingMessage{
			Type:    "status",
			Message: "Menunggu scan QR Code...",
		})

		loginSuccess := false
		loginTimer := time.NewTimer(160 * time.Second)
		defer loginTimer.Stop()

	qrLoop:
		for !loginSuccess {
			if client.Store.ID != nil {
				loginSuccess = true
				break
			}
			select {
			case <-ctx.Done():
				return
			case <-loginTimer.C:
				_ = wsjson.Write(ctx, c, WSOutgoingMessage{
					Type:    "error",
					Message: "Waktu scan QR habis (timeout). Silakan coba lagi.",
				})
				return
			case evt, ok := <-qrChan:
				if !ok {
					if client.Store.ID != nil {
						loginSuccess = true
					}
					break qrLoop
				}
				if evt.Event == "code" {
					// QR dikirim sebagai PNG dengan modul berukuran piksel bulat.
					qrImageBase64 := ""
					if pngBytes, qrErr := renderQRCode(evt.Code); qrErr != nil {
						log.Printf("[Session %s] Gagal membuat gambar QR: %v", sessionID, qrErr)
					} else {
						qrImageBase64 = "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes)
					}

					_ = wsjson.Write(ctx, c, WSOutgoingMessage{
						Type:    "qr",
						Code:    evt.Code,
						QRImage: qrImageBase64,
						Message: "Pindai kode QR menggunakan WhatsApp di ponsel Anda",
					})
				} else if evt.Event == "success" || client.Store.ID != nil {
					loginSuccess = true
					break qrLoop
				} else if evt.Event == "error" {
					_ = wsjson.Write(ctx, c, WSOutgoingMessage{
						Type:    "error",
						Message: fmt.Sprintf("Gagal scan QR: %v", evt.Error),
					})
					return
				} else {
					_ = wsjson.Write(ctx, c, WSOutgoingMessage{
						Type:    "status",
						Message: fmt.Sprintf("Status WhatsApp: %s", evt.Event),
					})
				}
			case <-connectedChan:
				loginSuccess = true
				break qrLoop
			case <-time.After(1 * time.Second):
				if client.Store.ID != nil {
					loginSuccess = true
					break qrLoop
				}
			}
		}

		// Verify login success
		if !loginSuccess && client.Store.ID == nil {
			_ = wsjson.Write(ctx, c, WSOutgoingMessage{
				Type:    "error",
				Message: "Sesi QR dibatalkan atau waktu habis.",
			})
			return
		}
	}

	// Login is successful!
	_ = wsjson.Write(ctx, c, WSOutgoingMessage{
		Type:    "status",
		Message: "Login WhatsApp berhasil! Menghubungkan sesi terotentikasi...",
	})

	log.Printf("[Session %s] Menunggu koneksi WhatsApp terotentikasi penuh pasca-pairing...", sessionID)
	if err := waitForAuthenticatedSession(ctx, client, connectedChan, loggedOutChan, 35*time.Second); err != nil {
		log.Printf("[Session %s] Gagal otentikasi pasca-login: %v", sessionID, err)
		_ = wsjson.Write(ctx, c, WSOutgoingMessage{
			Type:    "error",
			Message: fmt.Sprintf("Koneksi WhatsApp belum siap setelah login: %v. Silakan coba lagi.", err),
		})
		return
	}

	// Jeda singkat agar store dan cache protokol WhatsApp stabil sebelum request IQ
	time.Sleep(1 * time.Second)

	_ = wsjson.Write(ctx, c, WSOutgoingMessage{
		Type:    "status",
		Message: "Sedang mengunggah foto profil ukuran penuh (HD)...",
	})

	// Set group/profile picture
	_, err = client.SetGroupPhoto(ctx, types.EmptyJID, processedImg)
	if err != nil {
		_ = wsjson.Write(ctx, c, WSOutgoingMessage{
			Type:    "error",
			Message: fmt.Sprintf("Gagal memperbarui foto profil: %v", err),
		})
		return
	}

	// Immediately logout WhatsApp Web session for safety
	_ = wsjson.Write(ctx, c, WSOutgoingMessage{
		Type:    "status",
		Message: "Foto profil sukses terpasang! Mengeluarkan sesi WhatsApp otomatis demi keamanan...",
	})

	logoutErr := logoutSession(client, sessionID)
	if logoutErr == nil {
		loggedOut = true
		_ = wsjson.Write(ctx, c, WSOutgoingMessage{
			Type:    "success",
			Message: "Selamat! Foto profil berhasil diubah menjadi ukuran penuh (tanpa compress), dan sesi WhatsApp telah otomatis keluar (logged out).",
		})
	} else {
		log.Printf("[Session %s] Peringatan: sesi belum tercabut dari perangkat tertaut: %v", sessionID, logoutErr)
		_ = wsjson.Write(ctx, c, WSOutgoingMessage{
			Type:    "error",
			Message: "Foto profil berhasil dipasang, tetapi sesi WhatsApp belum tercabut dari Perangkat Tertaut. Buka WhatsApp di HP Anda lalu hapus perangkat secara manual.",
		})
	}

	_ = c.Close(websocket.StatusNormalClosure, "done")
}

// renderQRCode menghasilkan PNG QR code dengan lebar modul bilangan bulat.
//
// rsc.io/qr selalu memakai skala tetap 8 px per modul dan selalu memakai mask 0
// (lihat qr.go: "TODO: Pick appropriate mask"). Ukuran gambar hasilnya lalu
// diskalakan ulang oleh browser ke ukuran kotak QR, dan pembesaran non-integer
// membuat lebar modul tidak seragam sehingga kamera ponsel gagal membaca pola.
// Di sini kita encode ulang bitmap QR dengan skala eksplisit yang bulat supaya
// setiap modul menempati jumlah piksel yang sama persis.
func renderQRCode(payload string) ([]byte, error) {
	code, err := qr.Encode(payload, qr.L)
	if err != nil {
		return nil, fmt.Errorf("gagal encode QR: %w", err)
	}

	// Jumlah modul + quiet zone 4 modul di setiap sisi.
	const quietZone = 4
	// 10 px per modul menghasilkan PNG 690 px untuk QR 61 modul. Resolusi ini
	// tetap tajam di layar ber-DPR tinggi (retina) tanpa pembesaran ulang.
	const moduleScale = 10

	totalModules := code.Size + quietZone*2
	side := totalModules * moduleScale

	// Gambar grayscale 8-bit tanpa transparansi agar kontras hitam-putih maksimal.
	img := image.NewGray(image.Rect(0, 0, side, side))
	for i := range img.Pix {
		img.Pix[i] = 0xFF
	}
	offset := quietZone * moduleScale
	for y := 0; y < code.Size; y++ {
		for x := 0; x < code.Size; x++ {
			if !code.Black(x, y) {
				continue
			}
			for py := 0; py < moduleScale; py++ {
				rowStart := img.PixOffset(offset+x*moduleScale, offset+y*moduleScale+py)
				for px := 0; px < moduleScale; px++ {
					img.Pix[rowStart+px] = 0x00
				}
			}
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("gagal encode PNG QR: %w", err)
	}
	return buf.Bytes(), nil
}

// logoutSession attempts to cleanly unlink the companion device session from WhatsApp servers.
// If the websocket is not yet connected or during temporary reconnects, it reconnects and retries.
// Returns nil only when the unlink request was accepted by the server (or the session no longer exists).
func logoutSession(client *whatsmeow.Client, sessionID string) error {
	if client == nil || client.Store == nil {
		return errors.New("client atau device store tidak tersedia")
	}
	if client.Store.ID == nil || client.Store.GetJID().IsEmpty() {
		log.Printf("[Session %s] Sesi device ID sudah tidak ada di store, logout dianggap selesai.", sessionID)
		return nil
	}

	log.Printf("[Session %s] Memulai proses auto-logout dari WhatsApp...", sessionID)

	// Batas waktu total agar pemanggil (termasuk deferred cleanup) tidak menggantung.
	deadline := time.Now().Add(25 * time.Second)
	var lastErr error

	for attempt := 1; attempt <= 5; attempt++ {
		if time.Now().After(deadline) {
			lastErr = errors.New("batas waktu auto-logout habis")
			break
		}
		if client.Store == nil || client.Store.ID == nil {
			log.Printf("[Session %s] Device store sudah terhapus, logout dianggap selesai.", sessionID)
			return nil
		}

		// Pastikan websocket connected & logged in sebelum mengirim remove-companion-device.
		// Setelah pairing, whatsmeow melakukan restart stream (515) sehingga koneksi bisa
		// sedang tidak siap tepat saat logout diminta.
		if !client.IsConnected() || !client.IsLoggedIn() {
			log.Printf("[Session %s] Koneksi belum siap untuk logout, mencoba menyambung ulang (percobaan %d/5)...", sessionID, attempt)

			connectBudget := time.Until(deadline)
			if connectBudget > 8*time.Second {
				connectBudget = 8 * time.Second
			}
			if connectBudget < time.Second {
				lastErr = errors.New("batas waktu auto-logout habis sebelum menyambung")
				break
			}

			connectCtx, connectCancel := context.WithTimeout(context.Background(), connectBudget)
			err := client.ConnectContext(connectCtx)
			connectCancel()

			// ErrAlreadyConnected berarti socket masih hidup, cukup tunggu siap.
			if err != nil && !errors.Is(err, whatsmeow.ErrAlreadyConnected) {
				log.Printf("[Session %s] Gagal menyambung ulang untuk logout: %v", sessionID, err)
			}

			readyUntil := time.Now().Add(connectBudget)
			for time.Now().Before(readyUntil) {
				if client.IsConnected() && client.IsLoggedIn() {
					break
				}
				time.Sleep(150 * time.Millisecond)
			}
		}

		if !client.IsConnected() || !client.IsLoggedIn() {
			lastErr = errors.New("koneksi terotentikasi belum siap")
			log.Printf("[Session %s] Koneksi terotentikasi belum siap pada percobaan %d, mengulang...", sessionID, attempt)
			time.Sleep(500 * time.Millisecond)
			continue
		}

		requestBudget := time.Until(deadline)
		if requestBudget > 12*time.Second {
			requestBudget = 12 * time.Second
		}
		if requestBudget < time.Second {
			lastErr = errors.New("batas waktu auto-logout habis sebelum mengirim permintaan")
			break
		}

		logoutCtx, logoutCancel := context.WithTimeout(context.Background(), requestBudget)
		err := client.Logout(logoutCtx)
		logoutCancel()

		if err == nil {
			log.Printf("[Session %s] Berhasil logout dari server WhatsApp pada percobaan %d.", sessionID, attempt)
			return nil
		}

		lastErr = err
		log.Printf("[Session %s] Gagal logout pada percobaan %d: %v", sessionID, attempt, err)

		errMsg := strings.ToLower(err.Error())
		if errors.Is(err, whatsmeow.ErrNotLoggedIn) ||
			strings.Contains(errMsg, "device_removed") ||
			strings.Contains(errMsg, "logged out") ||
			strings.Contains(errMsg, "doesn't contain a device jid") {
			log.Printf("[Session %s] Sesi telah berhasil dicabut oleh server WhatsApp.", sessionID)
			return nil
		}

		// Server sempat menutup socket di tengah permintaan logout: pastikan sambung ulang.
		if !client.IsConnected() {
			log.Printf("[Session %s] Koneksi tertutup saat logout, menyambung ulang...", sessionID)
		}

		time.Sleep(500 * time.Millisecond)
	}

	log.Printf("[Session %s] Peringatan: auto-logout belum berhasil: %v", sessionID, lastErr)
	if lastErr == nil {
		lastErr = errors.New("auto-logout belum berhasil")
	}
	return fmt.Errorf("auto-logout gagal: %w", lastErr)
}

// waitForAuthenticatedSession menunggu hingga client WhatsApp terhubung dan terotentikasi penuh pasca-pairing.
// Fungsi ini menghindari race condition pada stream restart 515 dan tidak bergantung pada client.WaitForConnection
// yang mengembalikan false seketika saat expectedDisconnect aktif pasca-pairing.
func waitForAuthenticatedSession(
	ctx context.Context,
	client *whatsmeow.Client,
	connectedChan <-chan struct{},
	loggedOutChan <-chan struct{},
	timeout time.Duration,
) error {
	if client == nil {
		return errors.New("client is nil")
	}

	if client.IsConnected() && client.IsLoggedIn() {
		return nil
	}

	timeoutTimer := time.NewTimer(timeout)
	defer timeoutTimer.Stop()

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	// Jika koneksi sempat drop dan auto-reconnect tidak terpicu, bantu dengan Connect() manual
	nudgeTimer := time.NewTicker(2 * time.Second)
	defer nudgeTimer.Stop()

	for {
		if client.IsConnected() && client.IsLoggedIn() {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-timeoutTimer.C:
			return errors.New("waktu tunggu koneksi terotentikasi habis (timeout)")

		case <-loggedOutChan:
			return errors.New("sesi WhatsApp dikeluarkan atau dibatalkan dari ponsel")

		case <-connectedChan:
			if client.IsConnected() && client.IsLoggedIn() {
				return nil
			}
			time.Sleep(300 * time.Millisecond)
			return nil

		case <-nudgeTimer.C:
			if !client.IsConnected() && !client.IsLoggedIn() {
				log.Println("[whatsmeow] Membantu pemulihan koneksi via client.Connect()...")
				go func() {
					_ = client.Connect()
				}()
			}

		case <-ticker.C:
			if client.IsConnected() && client.IsLoggedIn() {
				return nil
			}
		}
	}
}

// processImage decodes base64, resizes using Lanczos fit to 535x720, and re-encodes as 100% JPEG
func processImage(dataURI string) ([]byte, error) {
	// Remove data URI prefix if present (e.g. data:image/jpeg;base64,)
	rawBase64 := dataURI
	if idx := strings.Index(dataURI, ","); idx != -1 {
		rawBase64 = dataURI[idx+1:]
	}

	imgBytes, err := base64.StdEncoding.DecodeString(rawBase64)
	if err != nil {
		return nil, fmt.Errorf("gagal decode base64: %w", err)
	}

	img, err := imaging.Decode(bytes.NewReader(imgBytes))
	if err != nil {
		return nil, fmt.Errorf("format gambar tidak didukung: %w", err)
	}

	const maxWidth = 535
	const maxHeight = 720

	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	// Only downscale if exceeds max resolution
	if w > maxWidth || h > maxHeight {
		img = imaging.Fit(img, maxWidth, maxHeight, imaging.Lanczos)
	}

	var buf bytes.Buffer
	err = imaging.Encode(&buf, img, imaging.JPEG, imaging.JPEGQuality(100))
	if err != nil {
		return nil, fmt.Errorf("gagal kompresi JPEG: %w", err)
	}

	return buf.Bytes(), nil
}
