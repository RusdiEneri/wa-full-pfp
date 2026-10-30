package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image/png"
	"testing"

	"rsc.io/qr"
)

// buildTestQRPayload meniru bentuk payload QR yang dikirim server WhatsApp:
// https://wa.me/settings/linked_devices#<ref>,<noise>,<identity>,<adv>,<clientType>
func buildTestQRPayload() string {
	key := func(fill byte) string {
		raw := bytes.Repeat([]byte{fill}, 32)
		return base64.StdEncoding.EncodeToString(raw)
	}
	return fmt.Sprintf(
		"https://wa.me/settings/linked_devices#2@VAHDREjnU6uDdLqZqmXgLzZBKydlPCMiEr4QZ4tHzQ6y3r9T4,%s,%s,%s,%s",
		key(0x11), key(0x22), key(0x33), "1",
	)
}

func TestRenderQRCode_ValidPNG(t *testing.T) {
	payload := buildTestQRPayload()

	data, err := renderQRCode(payload)
	if err != nil {
		t.Fatalf("renderQRCode gagal: %v", err)
	}

	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("hasil bukan PNG valid: %v", err)
	}

	code, err := qr.Encode(payload, qr.L)
	if err != nil {
		t.Fatalf("encode referensi gagal: %v", err)
	}

	// Setiap modul harus menempati jumlah piksel yang sama persis (bilangan bulat).
	totalModules := code.Size + 8
	bounds := img.Bounds()
	if bounds.Dx() != bounds.Dy() {
		t.Fatalf("gambar QR harus persegi, dapat %dx%d", bounds.Dx(), bounds.Dy())
	}
	if bounds.Dx()%totalModules != 0 {
		t.Fatalf("sisi gambar %d px tidak habis dibagi %d modul; modul tidak akan rata", bounds.Dx(), totalModules)
	}
}

func TestRenderQRCode_ModuleGridExactAndQuietZoneWhite(t *testing.T) {
	payload := buildTestQRPayload()

	data, err := renderQRCode(payload)
	if err != nil {
		t.Fatalf("renderQRCode gagal: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("hasil bukan PNG valid: %v", err)
	}

	code, err := qr.Encode(payload, qr.L)
	if err != nil {
		t.Fatalf("encode referensi gagal: %v", err)
	}

	const quietZone = 4
	totalModules := code.Size + quietZone*2
	side := img.Bounds().Dx()
	scale := side / totalModules
	if scale < 4 {
		t.Fatalf("skala modul terlalu kecil (%d px/modul), kamera ponsel sulit membaca QR", scale)
	}

	isDark := func(x, y int) bool {
		r, g, b, _ := img.At(x, y).RGBA()
		// Konversi ke luma kasar; PNG QR hanya berisi hitam dan putih.
		luma := (299*int(r) + 587*int(g) + 114*int(b)) / 1000
		return luma < 0x8000
	}

	// Quiet zone harus seluruhnya putih, kalau tidak pemindai gagal menemukan QR.
	for y := 0; y < quietZone*scale; y++ {
		for x := 0; x < side; x++ {
			if isDark(x, y) || isDark(x, side-1-y) {
				t.Fatalf("quiet zone tidak putih pada (%d,%d)", x, y)
			}
		}
	}

	// Setiap blok modul harus seragam: tidak boleh ada piksel campuran di dalam modul,
	// karena pembesaran non-integer inilah yang membuat QR sulit dipindai.
	for my := 0; my < code.Size; my++ {
		for mx := 0; mx < code.Size; mx++ {
			wantDark := code.Black(mx, my)
			baseX := (quietZone + mx) * scale
			baseY := (quietZone + my) * scale
			for py := 0; py < scale; py++ {
				for px := 0; px < scale; px++ {
					if got := isDark(baseX+px, baseY+py); got != wantDark {
						t.Fatalf("modul (%d,%d) tidak seragam pada offset (%d,%d): dapat %v, harus %v",
							mx, my, px, py, got, wantDark)
					}
				}
			}
		}
	}
}

func TestRenderQRCode_RejectsOversizedPayload(t *testing.T) {
	// Payload melampaui kapasitas maksimum QR (versi 40, level L) harus gagal dengan error,
	// bukan menghasilkan gambar rusak.
	huge := make([]byte, 5000)
	for i := range huge {
		huge[i] = 'A'
	}
	if _, err := renderQRCode(string(huge)); err == nil {
		t.Fatal("payload terlalu besar seharusnya mengembalikan error")
	}
}
