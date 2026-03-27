//go:build integration

package scraper_test

import (
	"strings"
	"testing"

	"github.com/imamputra1/idlix-downloader/internal/infrastructure/crypto"
	"github.com/imamputra1/idlix-downloader/internal/infrastructure/scraper"
)

func TestChromedpScraper_RealE2EPenetration(t *testing.T) {
	// TODO: Ganti dengan URL Film/Series Idlix yang VALID dan AKTIF hari ini
	targetURL := "https://tv12.idlixku.com/movie/peaky-blinders-the-immortal-man-2026/"

	// 1. Inisialisasi Modul
	domScraper := scraper.NewNativeHTTPScraper()
	aesDecrypter := crypto.NewAESDecrypter()

	// 2. Fase 1: Bypass WAF & Ekstraksi DOM
	t.Logf("Fase 1: Memulai penetrasi ke %s...", targetURL)
	scrapeRes := domScraper.ScraperMetadata(targetURL)

	if scrapeRes.IsErr() {
		_, err := scrapeRes.Unwrap()
		t.Fatalf("❌ Fase 1 Gagal (WAF Block/Network Error): %v", err)
	}

	metadata, _ := scrapeRes.Unwrap()
	t.Logf("✅ Fase 1 Berhasil! Post ID: %s", metadata.ID())
	t.Logf("🔑 Extracted Key: %s", metadata.DecryptionKey())

	// 3. Fase 2: Dekripsi Payload AES
	t.Log("Fase 2: Memulai dekripsi payload...")
	decryptRes := aesDecrypter.DecryptURL(metadata.EncryptedEmbedURL(), metadata.DecryptionKey())

	if decryptRes.IsErr() {
		_, err := decryptRes.Unwrap()
		t.Fatalf("❌ Fase 2 Gagal (Dekripsi Error): %v", err)
	}

	finalM3U8URL, _ := decryptRes.Unwrap()

	// 4. Asersi Hasil Akhir
	if finalM3U8URL == "" || !strings.HasPrefix(finalM3U8URL, "http") {
		t.Fatalf("❌ Hasil dekripsi bukan URL yang valid: %s", finalM3U8URL)
	}

	t.Logf("✅ PENETRASI TOTAL BERHASIL!")
	t.Logf("🎬 FINAL M3U8 URL: %s", finalM3U8URL)
}
