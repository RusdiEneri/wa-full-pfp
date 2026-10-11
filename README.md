---
title: WhatsApp Full Profile Picture
emoji: 📸
colorFrom: yellow
colorTo: red
sdk: gradio
app_file: app.py
pinned: true
license: mit
short_description: Pasang foto profil WhatsApp ukuran penuh tanpa terpotong (No Crop) dan resolusi tinggi HD.
---

# WhatsApp Full Profile Picture (No Crop HD)

Aplikasi web untuk memasang foto profil WhatsApp dalam resolusi penuh tanpa terpotong (No Crop).

Secara bawaan, aplikasi WhatsApp di ponsel memotong foto profil menjadi rasio persegi 1:1. Alat ini memanfaatkan protokol WhatsApp Web melalui pustaka [whatsmeow](https://github.com/tulir/whatsmeow) untuk mengunggah foto vertikal beresolusi hingga 535x720 piksel (rasio 3:4 atau 9:16) agar tampil utuh saat dilihat pada halaman info kontak.

---

## Konteks & Prinsip Keamanan

Aplikasi ini dirancang dengan prinsip privasi zero-retention dan multi-tenant:

1. **Sesi Bersifat Sementara (Ephemeral)**: Setiap permintaan diproses dalam sesi terisolasi menggunakan SQLite sementara (`sess_<uuid>.db`).
2. **Auto-Logout Seketika**: Begitu foto profil berhasil dipasang, server memanggil `client.Logout()` ke server WhatsApp secara otomatis. Sesi WhatsApp Web di ponsel Anda langsung terputus dan file sesi dihapus dari server.
3. **Tanpa Akses Data Obrolan**: Server hanya memanggil fungsi pembaruan avatar profil dan tidak membaca pesan, kontak, maupun riwayat percakapan.
4. **Proteksi Anti-Bot**: Dilindungi Cloudflare Turnstile untuk mencegah spam sesi dan menjaga kestabilan server.
5. **Desain Neubrutalism**: Antarmuka responsif berbasis Neubrutalism dengan kontras tinggi, navigasi taktil, serta dukungan tema Terang dan Gelap.

---

## Opsi Deployment

Aplikasi ini dapat dijalankan dalam beberapa arsitektur:

* **Arsitektur Terpisah (Rekomendasi Produksi Gratis)**:
  * Backend di **Hugging Face Spaces** (mendukung koneksi WebSocket persisten).
  * Frontend di **Vercel** (hosting statis cepat dengan custom domain).
* **Arsitektur Mandiri (All-in-One)**:
  * Backend dan frontend dijalankan bersamaan di komputer lokal atau VPS Docker.

---

### Cara 1: Deploy Backend ke Hugging Face Spaces

Repository ini sudah dilengkapi konfigurasi siap pakai untuk Hugging Face Spaces:

1. Buka [Hugging Face](https://huggingface.co/) dan buat Space baru di [huggingface.co/new-space](https://huggingface.co/new-space):
   * **Space name**: misalnya `wa-pfp-server`
   * **License**: `MIT`
   * **Space SDK**: Pilih **Gradio** (Template: **Blank**) atau **Docker**
   * **Space hardware**: Pilih **CPU basic (Free)**
2. Push repository ini ke Space Anda menggunakan Git:
   ```bash
   git remote add space https://huggingface.co/spaces/<username>/<space-name>
   git push space main
   ```
3. Mekanisme kerja di Hugging Face:
   * Jika menggunakan SDK Gradio, file `app.py` otomatis mendeteksi toolchain Go terbaru, mengompilasi binary server, mengaktifkan background keep-alive pinger, dan menjalankan server pada port 7860.
   * Jika menggunakan SDK Docker, `Dockerfile` multi-stage build akan menyusun container minimal berbasis Alpine dengan user non-root (UID 1000).
4. Setelah status Space berubah menjadi **Running**, endpoint backend Anda aktif di:
   ```
   https://<username>-<space-name>.hf.space
   ```

---

### Cara 2: Deploy Frontend ke Vercel

1. Buka [Vercel](https://vercel.com/) dan import repository GitHub Anda.
2. Pada bagian **Root Directory**, pilih folder:
   ```
   frontend
   ```
3. Klik **Deploy**.
4. Hubungkan Frontend ke Backend:
   * Buka website Vercel Anda di peramban.
   * Klik tombol **Pengaturan (ikon gear)** di pojok kanan atas.
   * Masukkan URL backend Hugging Face Anda (misal `https://username-spacename.hf.space`).
   * Pengaturan tersimpan di localStorage peramban dan status server akan berubah menjadi **Online**.

---

### Cara 3: Menjalankan di Komputer Lokal

1. Pastikan Golang versi 1.23+ sudah terpasang ([go.dev/dl](https://go.dev/dl/)).
2. Unduh dependensi:
   ```bash
   go mod tidy
   ```
3. Jalankan server:
   ```bash
   go run .
   ```
4. Buka peramban dan akses:
   ```
   http://localhost:7860
   ```

---

### Cara 4: Menjalankan dengan Docker

```bash
docker build -t wa-full-pfp .
docker run -p 7860:7860 wa-full-pfp
```

---

## Konfigurasi Cloudflare Turnstile

Untuk mengaktifkan proteksi bot pada form:

1. Buka Cloudflare Dashboard > menu **Turnstile**.
2. Daftarkan domain frontend Anda (misal `ppfull.mangrusdi.my.id`, domain Vercel, dan `localhost`).
3. Pilih mode widget **Managed**.
4. Tambahkan environment variable pada server backend (misal di Settings > Variables and secrets Hugging Face Spaces):
   * `TURNSTILE_SECRET_KEY`: Secret key dari Cloudflare.
   * `TURNSTILE_SITE_KEY`: Site key publik dari Cloudflare.
5. Frontend otomatis mengambil `turnstile_site_key` via endpoint `/config` dan merender verifikasi keamanan sebelum pengunggahan foto diizinkan.

> Catatan: Jika `TURNSTILE_SECRET_KEY` tidak diisi (misal saat pengujian lokal), backend otomatis beroperasi dalam mode bypass untuk memudahkan pengembangan.

---

## Struktur Proyek

```
.
|-- Dockerfile              # Container image untuk Hugging Face Docker SDK
|-- app.py                  # Bootstrap launcher & keep-alive daemon Hugging Face
|-- main.go                 # HTTP server, WebSocket handler, whatsmeow session logic
|-- turnstile_test.go       # Unit test validasi Turnstile & ekstraksi IP
|-- logout_test.go          # Unit test penanganan disconnect & cleanup sesi
|-- frontend/               # Antarmuka web statis (HTML, Vanilla CSS, JS)
|   |-- index.html          # Halaman utama dengan Schema.org & Meta SEO
|   |-- 404.html            # Halaman error 404 bertema Neubrutalism
|   |-- style.css           # Sistem desain Neubrutalism & responsif mobile
|   |-- app.js              # Interaksi UI, WebSocket client, Turnstile lifecycle
|   |-- robots.txt          # Aturan perayapan mesin pencari & link sitemap
|   |-- sitemap.xml         # Peta situs terstruktur
|   `-- site.webmanifest    # Konfigurasi PWA dan ikon web
`-- README.md
```

---

## Lisensi & Atribusi

* Dilisensikan di bawah [MIT License](LICENSE).
* Protokol komunikasi WhatsApp menggunakan [whatsmeow](https://github.com/tulir/whatsmeow) oleh Tulir Asokan.
* Alat ini merupakan utilitas pihak ketiga independen dan tidak berafiliasi dengan WhatsApp Inc. maupun Meta Platforms.
