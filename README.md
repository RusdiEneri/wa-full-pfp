---
title: WhatsApp Full Profile Picture
emoji: 📸
colorFrom: green
colorTo: blue
sdk: gradio
app_file: app.py
pinned: true
---

# WhatsApp Full Profile Picture (No-Crop HD) Web App

Aplikasi web modern untuk mengubah foto profil WhatsApp menjadi **ukuran penuh (No-Crop)** dan **resolusi tinggi (HD tanpa kompresi buram)**.

Dibangun dengan arsitektur **multi-tenant** yang aman:
* **Sesi WhatsApp bersifat sementara (ephemeral)** diisolasi per pengguna.
* **Auto-Logout Otomatis:** Begitu foto profil berhasil dipasang, sesi WhatsApp Web langsung di-logout otomatis dari perangkat Anda dan data sementara dihapus seketika dari server.

---

## 🚀 Panduan Deployment

Aplikasi ini dirancang untuk dapat dijalankan secara terpisah:
1. **Backend** di **Hugging Face Spaces** (Docker Go gratis, mendukung WebSocket persistent).
2. **Frontend** di **Vercel** (UI statis, gratis & cepat).
3. Atau **Langsung dijalankan lokal** di laptop Anda (Backend + Frontend langsung jalan bersamaan).

---

### Cara 1: Deploy Backend ke Hugging Face Spaces (100% Gratis)

1. Buka [Hugging Face](https://huggingface.co/) & buat Space baru ([huggingface.co/new-space](https://huggingface.co/new-space)):
   - **Space name**: misalnya `wa-pfp-server`
   - **Select the Space SDK**: Pilih **Gradio** (Template: **Blank**) *(100% Gratis!)*
   - **Space hardware**: Pilih **ZeroGPU (Free)**
2. Push repository ini ke Hugging Face Space Anda (via Git).
3. Hugging Face akan menjalankan launcher [`app.py`](app.py) yang otomatis mengompilasi dan menjalankan server Go pada port 7860.
4. Setelah statusnya **Running**, backend Anda aktif di:
   `https://<username>-<spacename>.hf.space`

---

### Cara 2: Deploy Frontend ke Vercel (Gratis)

1. Buka [Vercel](https://vercel.com/) dan buat project baru dari repository GitHub Anda.
2. Pada bagian **Root Directory**, klik **Edit** dan pilih folder:
   ```
   frontend
   ```
3. Klik **Deploy**. Vercel akan langsung meng-online-kan website Anda dalam hitungan detik.
4. **Menghubungkan ke Backend:**
   - Buka website Vercel Anda.
   - Klik ikon **Pengaturan (Gear)** atau tombol **"Backend Offline / Memeriksa..."** di pojok kanan atas.
   - Masukkan URL Hugging Face Space Anda (contoh: `https://username-spacename.hf.space`).
   - Status akan langsung berubah menjadi **Backend Online (Hijau)**.

---

### Cara 3: Menjalankan di Laptop Lokal (Development / Pribadi)

Anda juga bisa menjalankan seluruh web app (Backend + Frontend) secara langsung di komputer/laptop:

1. **Install Golang**: Unduh installer resmi dari [go.dev/dl](https://go.dev/dl/).
2. **Unduh dependencies**:
   ```powershell
   go mod tidy
   ```
3. **Jalankan server**:
   ```powershell
   go run .
   ```
4. Buka browser dan kunjungi:
   ```
   http://localhost:7860
   ```
   Website siap digunakan secara offline/lokal!

---

## 🛡️ Cloudflare Turnstile (Perlindungan Anti-Bot)

Aplikasi ini dilengkapi dengan **Cloudflare Turnstile** untuk mencegah serangan bot, spam pembuatan sesi WhatsApp, dan menjaga server Hugging Face Spaces tetap stabil.

### Cara Mengaktifkan Cloudflare Turnstile:
1. Buka [Cloudflare Dashboard](https://dash.cloudflare.com/) > menu **Turnstile**.
2. Klik **Add site**:
   - **Site name**: `WA Full Profile Picture`
   - **Domain**: Masukkan domain frontend Vercel Anda (misal `wa-pfp.vercel.app`) dan domain lokal `localhost`.
   - **Widget Mode**: Pilih **Managed** (Sangat direkomendasikan - verifikasi pintar tanpa tebak-gambar rumit).
3. Salin **Site Key** dan **Secret Key**.
4. Di Hugging Face Space Anda:
   - Buka menu **Settings** > **Variables and secrets**.
   - Tambahkan **Secret**:
     - `TURNSTILE_SECRET_KEY` = *(Secret Key Anda)*
   - Tambahkan **Variable**:
     - `TURNSTILE_SITE_KEY` = *(Site Key Anda)*
5. **Selesai!** Frontend di Vercel secara otomatis mendeteksi konfigurasi tersebut via endpoint `/config` dari Hugging Face dan langsung merender widget Turnstile di atas tombol proses tanpa perlu redeploy Vercel.

> 💡 **Mode Dev / Pengujian Lokal:** Jika `TURNSTILE_SECRET_KEY` dikosongkan (misal saat dijalankan di komputer lokal), backend secara otomatis masuk ke **Dev/Bypass Mode** sehingga Anda tetap bisa melakukan pengujian tanpa harus menyiapkan akun Cloudflare terlebih dahulu.

---

## 🔒 Privasi & Keamanan

1. **Tidak Ada Database Permanen**: Menggunakan SQLite sementara (`file:sess_<uuid>.db`) yang dihapus secara permanen seketika proses selesai.
2. **Auto-Logout Seketika**: Kode secara eksplisit memanggil `client.Logout()` ke server WhatsApp setelah foto berhasil diunggah, memastikan sesi tertaut di HP Anda langsung terputus.
3. **Perlindungan Anti-Bot**: Dilindungi Cloudflare Turnstile dengan validasi token *single-use* sebelum sesi WhatsApp whatsmeow dialokasikan.
4. **Tanpa Akses Chat**: Aplikasi ini hanya memanggil API `SetGroupPhoto` (untuk foto profil) dan tidak membaca kontak maupun riwayat obrolan Anda.

---

## 🛠️ Teknologi yang Digunakan
* **Backend:** Go, [whatsmeow](https://github.com/tulir/whatsmeow), [glebarez/go-sqlite](https://github.com/glebarez/go-sqlite) (Pure Go tanpa CGO/GCC), [imaging](https://github.com/disintegration/imaging), [coder/websocket](https://github.com/coder/websocket).
* **Frontend:** HTML5, Vanilla CSS (Glassmorphism Dark Theme), Vanilla JavaScript.
* **Hosting Support:** Hugging Face Spaces (Docker), Vercel.
