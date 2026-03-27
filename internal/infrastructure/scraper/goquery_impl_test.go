//go:build integration

package scraper_test

import (
	"net/http/cookiejar"
	"testing"

	"github.com/imamputra1/idlix-downloader/internal/infrastructure/client"
	"github.com/imamputra1/idlix-downloader/internal/infrastructure/scraper"
)

func TestGoQueryScraper_RealE2EPenetration(t *testing.T) {
	// TODO: Ganti URL ini dengan URL agregator (Idlix) yang valid dan aktif.
	targetURL := "https://tv12.idlixku.com/movie/peaky-blinders-the-immortal-man-2026/"

	// 1. Dependency Injection: Injeksi AntiBot uTLS (Task 6)
	antiBotClient := client.NewAntiBotClient()

	// Kombinasi Cookie + Nonce adalah kunci mutlak menembus Honeypot WAF mereka!
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("Gagal membuat cookie jar: %v", err)
	}
	antiBotClient.Jar = jar
	// =========================================================================

	// Memasukkan client yang sudah memiliki "ingatan cookie" ke dalam scraper
	domScraper := scraper.NewGoQueryScraper(antiBotClient)

	// 2. Eksekusi Ekstraksi E2E
	t.Logf("Memulai penetrasi E2E ke: %s", targetURL)
	res := domScraper.ScraperMetadata(targetURL)

	if res.IsErr() {
		_, err := res.Unwrap()
		t.Fatalf("Penetrasi/Ekstraksi Gagal (WAF Block/Network Error): %v", err)
	}

	metadata, err := res.Unwrap()
	if err != nil {
		t.Fatalf("Unexpected error unwrapping result: %v", err)
	}

	// 3. Asersi Fisik: Memastikan artefak benar-benar dikembalikan dari server asli
	if metadata.ID() == "" {
		t.Error("E2E Failed: Post ID kosong. Kemungkinan struktur DOM target berubah (Selektor CSS usang).")
	}

	if metadata.EncryptedEmbedURL() == "" {
		t.Error("E2E Failed: Ciphertext kosong. Kemungkinan AJAX Endpoint berubah atau payload diblokir.")
	}

	// Logging sukses untuk memvalidasi panjang ciphertext yang ditangkap
	t.Logf("E2E Berhasil! ID Target: %s | Panjang Ciphertext: %d bytes", metadata.ID(), len(metadata.EncryptedEmbedURL()))
}
