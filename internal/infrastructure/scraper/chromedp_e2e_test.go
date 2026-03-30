//go:build integration

package scraper_test

import (
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/imamputra1/idlix-downloader/internal/infrastructure/crypto"
	"github.com/imamputra1/idlix-downloader/internal/infrastructure/scraper"
)

func TestChromedpScraper_RealE2EPenetration(t *testing.T) {
	targetURL := "https://tv12.idlixku.com/movie/peaky-blinders-the-immortal-man-2026/"

	domScraper := scraper.NewNativeHTTPScraper()
	aesDecrypter := crypto.NewAESDecrypter()

	t.Logf("Fase 1: Memulai penetrasi ke %s...", targetURL)
	scrapeRes := domScraper.ScraperMetadata(targetURL)

	if scrapeRes.IsErr() {
		_, err := scrapeRes.Unwrap()
		t.Fatalf("Fase 1 Gagal (WAF Block/Network Error): %v", err)
	}

	metadata, _ := scrapeRes.Unwrap()
	t.Logf("Fase 1 Berhasil! Post ID: %s", metadata.ID())
	t.Logf("Extracted Key: %s", metadata.DecryptionKey())

	t.Log("Fase 2: Memulai dekripsi payload...")
	decryptRes := aesDecrypter.DecryptURL(metadata.EncryptedEmbedURL(), metadata.DecryptionKey())

	if decryptRes.IsErr() {
		_, err := decryptRes.Unwrap()
		t.Fatalf("Fase 2 Gagal (Dekripsi Error): %v", err)
	}

	finalURL, _ := decryptRes.Unwrap()

	if finalURL == "" || !strings.HasPrefix(finalURL, "http") {
		t.Fatalf("Hasil dekripsi bukan URL yang valid: %s", finalURL)
	}

	t.Logf("PENETRASI TOTAL BERHASIL!")
	t.Logf("FINAL M3U8 URL: %s", finalURL)

	t.Log("Fase 4: Mengekstrak Video Source melalui API JeniusPlay...")

	parts := strings.Split(strings.TrimRight(finalURL, "/"), "/")
	hash := parts[len(parts)-1]

	jeniusAPI := "https://jeniusplay.com/player/index.php?data=" + hash + "&do=getVideo"
	payload := "hash=" + hash + "&r=" + targetURL

	apiReq, err := http.NewRequest(http.MethodPost, jeniusAPI, strings.NewReader(payload))
	if err != nil {
		t.Fatalf("Gagal membuat request API Jenius: %v", err)
	}

	apiReq.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	apiReq.Header.Set("X-Requested-With", "XMLHttpRequest")
	apiReq.Header.Set("Referer", finalURL)
	apiReq.Header.Set("Origin", "https://jeniusplay.com")
	apiReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	apiReq.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")

	client := &http.Client{}
	apiResp, err := client.Do(apiReq)
	if err != nil {
		t.Fatalf("Network error Fase 4: %v", err)
	}
	defer apiResp.Body.Close()

	if apiResp.StatusCode != 200 {
		t.Fatalf("Fase 4 ditolak server JeniusPlay, Status: %d", apiResp.StatusCode)
	}

	apiBytes, _ := io.ReadAll(apiResp.Body)

	var jeniusResponse struct {
		VideoSource string `json:"videoSource"`
	}

	videoSourceRegex := regexp.MustCompile(`"videoSource"\s*:\s*"([^"]+)"`)
	match := videoSourceRegex.FindStringSubmatch(string(apiBytes))

	if len(match) > 1 {
		jeniusResponse.VideoSource = strings.ReplaceAll(match[1], `\/`, `/`)
	}

	if jeniusResponse.VideoSource == "" {
		t.Fatalf("Gagal menemukan videoSource di dalam JSON JeniusPlay. Raw Response: %s", string(apiBytes))
	}

	masterM3U8 := jeniusResponse.VideoSource
	if lastDot := strings.LastIndex(masterM3U8, "."); lastDot != -1 {
		masterM3U8 = masterM3U8[:lastDot] + ".m3u8"
	}

	t.Log("==================================================")
	t.Logf("MASTER M3U8 ASLI DITEMUKAN: %s", masterM3U8)
	t.Log("==================================================")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
