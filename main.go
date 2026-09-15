package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
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
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
	"rsc.io/qr"
)

//go:embed all:frontend
var embeddedFrontend embed.FS

// WSIncomingMessage represents requests sent by the frontend
type WSIncomingMessage struct {
	Action     string `json:"action"`      // "start"
	Image      string `json:"image"`       // Base64 data URL
	PairNumber string `json:"pair_number"` // Optional phone number for pairing code
}

// WSOutgoingMessage represents messages sent to the frontend
type WSOutgoingMessage struct {
	Type    string `json:"type"`               // "status", "qr", "pairing_code", "success", "error"
	Message string `json:"message,omitempty"`  // Status description
	Code    string `json:"code,omitempty"`     // Raw QR or Pairing code
	QRImage string `json:"qr_image,omitempty"` // Base64 PNG data URL of QR code
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

	// WebSocket handler for WhatsApp session
	mux.HandleFunc("/ws", handleWebSocket)

	// Static frontend handler (embedded)
	frontendSub, err := fs.Sub(embeddedFrontend, "frontend")
	if err == nil {
		fileServer := http.FileServer(http.FS(frontendSub))
		mux.Handle("/", fileServer)
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
	defer func() {
		log.Printf("[Session %s] Membersihkan sesi...", sessionID)
		if client != nil && client.IsConnected() {
			client.Disconnect()
		}
		_ = container.Close()
		_ = os.Remove(dbFile)
		_ = os.Remove(dbFile + "-wal")
		_ = os.Remove(dbFile + "-shm")
		log.Printf("[Session %s] Sesi selesai dan dibersihkan.", sessionID)
	}()

	// Device properties (mimic Google Chrome on Windows dengan versi terbaru)
	store.DeviceProps.PlatformType = waCompanionReg.DeviceProps_CHROME.Enum()
	store.DeviceProps.Os = proto.String("Google Chrome (Windows)")
	
	// PENTING: Set versi WhatsApp Web terbaru agar tidak ditolak server karena dianggap "kadaluarsa" (outdated)
	store.DeviceProps.Version = &waCompanionReg.DeviceProps_AppVersion{
		Primary:   2,
		Secondary: 3000,
		Tertiary:  1026, // Versi WhatsApp Web terkini (2.3000.1026)
	}

	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		_ = wsjson.Write(ctx, c, WSOutgoingMessage{
			Type:    "error",
			Message: "Gagal membuat device store.",
		})
		return
	}

	clientLog := waLog.Stdout("Client", "ERROR", true)
	client = whatsmeow.NewClient(deviceStore, clientLog)

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

		qrCtx, qrCancel := context.WithCancel(ctx)
		defer qrCancel()

		qrChan, err := client.GetQRChannel(qrCtx)
		if err != nil {
			_ = wsjson.Write(ctx, c, WSOutgoingMessage{
				Type:    "error",
				Message: fmt.Sprintf("Gagal menyiapkan channel pairing: %v", err),
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

		// Tunggu QR pertama sebagai indikator websocket login sudah established
		select {
		case evt, ok := <-qrChan:
			if !ok {
				_ = wsjson.Write(ctx, c, WSOutgoingMessage{
					Type:    "error",
					Message: "Channel WhatsApp ditutup sebelum koneksi siap.",
				})
				return
			}
			if evt.Event != "code" {
				_ = wsjson.Write(ctx, c, WSOutgoingMessage{
					Type:    "error",
					Message: fmt.Sprintf("WhatsApp belum siap untuk pairing (event: %s).", evt.Event),
				})
				return
			}
			log.Printf("[Session %s] WhatsApp websocket siap untuk PairPhone (IsConnected=%v)",
				sessionID, client.IsConnected())
		case <-ctx.Done():
			return
		}

		// Tetap drain QR channel sampai pairing selesai
		go func() {
			for evt := range qrChan {
				log.Printf("[Session %s] Pairing QR event: %s", sessionID, evt.Event)
			}
			log.Printf("[Session %s] QR channel ditutup", sessionID)
		}()

		// PairPhone dipanggil SEGERA setelah QR pertama diterima.
		// Gunakan "Google Chrome (Windows)" yang merupakan format canonical yang diterima server WhatsApp
		code, err := client.PairPhone(ctx, pairNumber, true, whatsmeow.PairClientChrome, "Google Chrome (Windows)")
		if err != nil {
			errMsg := err.Error()
			if strings.Contains(errMsg, "400") || strings.Contains(errMsg, "bad-request") {
				errMsg = "Permintaan ditolak oleh server WhatsApp (400). Ini biasanya terjadi jika: 1) Anda menjalankan ini di IP Cloud/Datacenter (seperti HuggingFace/Vercel) yang diblokir WhatsApp, 2) Nomor tidak terdaftar di WhatsApp, atau 3) IP Anda terdeteksi sebagai bot."
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

		// Wait for login or timeout (2 minutes)
		loginSuccess := false
		for i := 0; i < 120; i++ {
			if client.Store.ID != nil {
				loginSuccess = true
				break
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(1 * time.Second):
			}
		}

		if !loginSuccess {
			_ = wsjson.Write(ctx, c, WSOutgoingMessage{
				Type:    "error",
				Message: "Waktu pairing habis (timeout). Silakan coba lagi.",
			})
			return
		}

	} else {
		// Mode 2: Scan QR Code
		qrChan, _ := client.GetQRChannel(ctx)

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

		// Loop QR events
		for evt := range qrChan {
			if evt.Event == "code" {
				// Generate QR Code PNG image
				qrImageBase64 := ""
				qrCodeObj, qrErr := qr.Encode(evt.Code, qr.L)
				if qrErr == nil {
					pngBytes := qrCodeObj.PNG()
					qrImageBase64 = "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes)
				}

				_ = wsjson.Write(ctx, c, WSOutgoingMessage{
					Type:    "qr",
					Code:    evt.Code,
					QRImage: qrImageBase64,
					Message: "Pindai kode QR menggunakan WhatsApp di ponsel Anda",
				})
			} else {
				_ = wsjson.Write(ctx, c, WSOutgoingMessage{
					Type:    "status",
					Message: fmt.Sprintf("Status WhatsApp: %s", evt.Event),
				})
			}
		}

		// Verify login success
		if client.Store.ID == nil {
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
		Message: "Login WhatsApp berhasil! Menyiapkan pemasangan foto profil...",
	})

	// Small pause so WhatsApp internal stores settle
	time.Sleep(2500 * time.Millisecond)

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

	logoutCtx, logoutCancel := context.WithTimeout(context.Background(), 5*time.Second)
	_ = client.Logout(logoutCtx)
	logoutCancel()

	_ = wsjson.Write(ctx, c, WSOutgoingMessage{
		Type:    "success",
		Message: "Selamat! Foto profil berhasil diubah menjadi ukuran penuh (tanpa compress), dan sesi WhatsApp telah otomatis keluar (logged out).",
	})

	_ = c.Close(websocket.StatusNormalClosure, "done")
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
