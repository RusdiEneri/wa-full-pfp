# Desain Teknis: Integrasi Cloudflare Turnstile pada WhatsApp Full Profile Picture

**Tanggal:** 2026-10-07  
**Status:** Disetujui  
**Target Platform:** Backend (Hugging Face Spaces - Go), Frontend (Vercel - Vanilla JS/HTML/CSS)

---

## 1. Latar Belakang & Tujuan
Aplikasi **WhatsApp Full Profile Picture (ppfull)** memungkinkan pengguna mengunggah foto profil tanpa pemotongan (no-crop) dan menginstalnya langsung ke akun WhatsApp via session pairing / QR scan. 
Untuk mencegah penyalahgunaan resource server, spam sesi WhatsApp, dan potensi serangan bot/DDoS terhadap backend di Hugging Face Spaces, sistem memerlukan mekanisme verifikasi bot modern dan ramah pengguna.
Dipilih **Cloudflare Turnstile** karena:
- Menghadirkan perlindungan tangguh tanpa membebani pengguna dengan puzzle/gambar CAPTCHA usang.
- Menggunakan mode *Managed* yang cepat dan seringkali lolos secara otomatis (smart invisible verification).
- Gratis dan didukung penuh oleh infrastruktur global Cloudflare.

---

## 2. Arsitektur Solusi

### 2.1 Alur Komunikasi Sistem
```
+-------------------------------------------------------------+
|                     Frontend (Vercel)                       |
+-------------------------------------------------------------+
   | (1) GET /config (ambil TURNSTILE_SITE_KEY)
   v
+-------------------------------------------------------------+
|                 Backend Go (Hugging Face)                   |
+-------------------------------------------------------------+
   | (2) Response: { turnstile_enabled: true, turnstile_site_key: "..." }
   v
+-------------------------------------------------------------+
|               Frontend Render Turnstile Widget              |
+-------------------------------------------------------------+
   | (3) Interaksi user -> Cloudflare Turnstile API
   v
+-------------------------------------------------------------+
|               Cloudflare Turnstile Service                  |
+-------------------------------------------------------------+
   | (4) Menghasilkan Turnstile Token ke Frontend
   v
+-------------------------------------------------------------+
|            Frontend Kirim WebSocket { action: "start" }     |
|              menyertakan payload `turnstile_token`          |
+-------------------------------------------------------------+
   | (5) WebSocket Connect & Send Init Message
   v
+-------------------------------------------------------------+
|                 Backend Go (Hugging Face)                   |
|       Memverifikasi token ke Cloudflare Siteverify API      |
+-------------------------------------------------------------+
   | (6) POST https://challenges.cloudflare.com/turnstile/v0/siteverify
   v
+-------------------------------------------------------------+
|               Cloudflare Turnstile Siteverify               |
+-------------------------------------------------------------+
   | (7) Response: { "success": true }
   v
+-------------------------------------------------------------+
|                 Backend Loloskan Verifikasi                 |
|       Inisialisasi Whatsmeow & Sesi WhatsApp Dimulai        |
+-------------------------------------------------------------+
```

---

## 3. Spesifikasi Komponen

### 3.1 Backend Go (`main.go`)

#### Variabel Lingkungan (*Environment Variables*)
- `TURNSTILE_SITE_KEY`: Public site key dari Cloudflare Dashboard.
- `TURNSTILE_SECRET_KEY`: Secret key dari Cloudflare Dashboard.

#### Endpoint `GET /config`
- Menangani permintaan konfigurasi publik dari frontend.
- Mengembalikan response JSON:
  ```json
  {
    "turnstile_enabled": true,
    "turnstile_site_key": "0x4AAAAAA..."
  }
  ```
- **Aturan Mode**:
  - `turnstile_enabled` adalah `true` jika `TURNSTILE_SITE_KEY` dan `TURNSTILE_SECRET_KEY` tidak kosong.
  - Jika `TURNSTILE_SECRET_KEY` kosong, `turnstile_enabled` bernilai `false` (Dev/Bypass Mode).

#### Pembaruan Struct Payload WebSocket
```go
type WSIncomingMessage struct {
    Action         string `json:"action"`          // "start"
    Image          string `json:"image"`           // Base64 data URL
    PairNumber     string `json:"pair_number"`     // Opsional
    TurnstileToken string `json:"turnstile_token"` // Token Turnstile
}
```

#### Logika Verifikasi Token (`verifyTurnstileToken`)
```go
type TurnstileVerifyResponse struct {
    Success     bool      `json:"success"`
    ChallengeTS time.Time `json:"challenge_ts"`
    Hostname    string    `json:"hostname"`
    ErrorCodes  []string  `json:"error-codes"`
    Action      string    `json:"action"`
    Cdata       string    `json:"cdata"`
}
```
- **URL Verifikasi**: `https://challenges.cloudflare.com/turnstile/v0/siteverify`
- **Metode**: `POST` dengan `application/x-www-form-urlencoded`
  - Parameter: `secret`, `response` (token), `remoteip` (opsional).
- **Aturan Keputusan**:
  1. Jika `TURNSTILE_SECRET_KEY` tidak disetel: Log warning `[Turnstile] Secret key kosong, verifikasi di-bypass untuk dev`, kembalikan `nil` (lolos).
  2. Jika `turnstile_token` kosong saat `TURNSTILE_SECRET_KEY` aktif: Tolak koneksi.
  3. Jika panggilan HTTP gagal atau Cloudflare mengembalikan `success: false`: Tolak koneksi dan kirim pesan error:
     `Verifikasi keamanan Turnstile gagal atau kedaluwarsa. Silakan muat ulang halaman.`
  4. Jika `success: true`: Lanjutkan pemrosesan sesi Whatsmeow.

---

### 3.2 Frontend Vercel (`frontend/`)

#### 1. `frontend/index.html`
- Memuat skrip Cloudflare Turnstile di `<head>` dengan mode `render=explicit`:
  ```html
  <script src="https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit" async defer></script>
  ```
- Menambahkan kontainer widget di dalam formulir upload tepat sebelum tombol aksi:
  ```html
  <div id="turnstile-wrapper" class="turnstile-wrapper hidden">
    <div id="turnstile-widget"></div>
    <div id="turnstile-status" class="turnstile-note"></div>
  </div>
  ```

#### 2. `frontend/app.js`
- **State Management**:
  - Menyimpan `turnstileWidgetId: null`, `turnstileToken: null`, `turnstileEnabled: false`.
- **Inisialisasi**:
  - Setelah `checkBackendHealth()` berhasil, panggil `fetchConfig()`.
  - Jika backend mengembalikan `turnstile_enabled: true` dan ada `turnstile_site_key`:
    - Tampilkan `#turnstile-wrapper`.
    - Panggil `turnstile.render('#turnstile-widget', { sitekey, callback, 'expired-callback', 'error-callback', theme: 'dark' })`.
  - Jika `turnstile_enabled: false`:
    - Sembunyikan `#turnstile-wrapper` dan izinkan proses berjalan langsung.
- **Validasi Tombol "Mulai Hubungkan"**:
  - Tombol aktif jika `state.currentImageBase64` ada DAN (`!state.turnstileEnabled` ATAU `state.turnstileToken` valid).
- **Pengiriman Pesan WebSocket**:
  - Sertakan `turnstile_token: state.turnstileToken` dalam payload `{ action: 'start' }`.
  - Setelah proses dimulai atau jika error, reset widget turnstile agar token tidak dipakai ulang (*one-time use*).

#### 3. `frontend/style.css`
- Menata `#turnstile-wrapper` dengan padding, border glassmorphism, dan alignment tengah yang rapi dan elegan sesuai tema *dark mode*.

---

## 4. Keamanan & Penanganan Edge Cases

| Skenario | Penanganan |
|---|---|
| Token Kedaluwarsa (Expired) | Turnstile `expired-callback` otomatis mengosongkan state token dan meminta user mengklik/verifikasi ulang widget. |
| Replay Attack (Token Dipakai Ulang) | Cloudflare API otomatis menolak token yang sudah pernah divalidasi. Backend menolak request dan memutus sesi. |
| Backend Offline / Sleeping di Hugging Face | Widget tidak dirender hingga backend bangun (`/health` & `/config` sukses 200). Tombol aksi dinonaktifkan dengan status *"Membangunkan Server..."*. |
| Local Development (Tanpa Akun Cloudflare) | Tanpa mengisi `TURNSTILE_SECRET_KEY` di `.env`, backend otomatis aktif dalam Dev Mode (bypassed), pengembang tetap bisa coding lokal tanpa error. |

---

## 5. Rencana Pengujian
1. **Unit Test Go**:
   - `main_test.go` untuk menguji fungsi `verifyTurnstileToken` (kondisi bypass, token kosong, payload mock).
   - Test endpoint `GET /config` untuk memastikan JSON response sesuai format.
2. **Build Verification**:
   - Menjalankan `go build` untuk memastikan tidak ada kesalahan kompilasi.
3. **Integrasi & Pengujian Browser**:
   - Verifikasi bahwa frontend memuat widget jika key disediakan.
   - Verifikasi bahwa backend menerima token melalui WebSocket dan memprosesnya dengan benar.
