package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/imamputra1/idlix-downloader/internal/infrastructure/crypto"
	"github.com/imamputra1/idlix-downloader/internal/infrastructure/downloader"
	"github.com/imamputra1/idlix-downloader/internal/infrastructure/scraper"
)

func main() {
	// 1. Setup Argumen Command Line
	urlPtr := flag.String("url", "", "URL Film atau Series dari Idlix")
	outPtr := flag.String("out", "movie.mp4", "Nama file output (contoh: spiderman.mp4)")
	workersPtr := flag.Int("workers", 10, "Jumlah Goroutine (kecepatan unduh paralel)")
	flag.Parse()

	if *urlPtr == "" {
		fmt.Println("Error: URL wajib diisi!")
		fmt.Println("Gunakan: go run cmd/idlix-dl/main.go -url=\"https://...\" -out=\"video.mp4\"")
		os.Exit(1)
	}

	targetURL := *urlPtr
	outputFile := *outPtr

	fmt.Println("==================================================")
	fmt.Println("IDLIX DOWNLOADER ENGINE - V1.0")
	fmt.Println("==================================================")

	// 2. Inisialisasi Modul
	domScraper := scraper.NewNativeHTTPScraper()
	aesDecrypter := crypto.NewAESDecrypter()
	hlsDownloader := downloader.NewHSLDownloader(*workersPtr)

	// 3. FASE 1: Scraping DOM & Bypass Cloudflare
	fmt.Printf("[1/4] Menembus pertahanan WAF dan mengekstrak Payload... (%s)\n", targetURL)
	scrapeRes := domScraper.ScraperMetadata(targetURL)
	if scrapeRes.IsErr() {
		_, err := scrapeRes.Unwrap()
		fmt.Printf("Fase 1 Gagal: %v\n", err)
		os.Exit(1)
	}
	metadata, _ := scrapeRes.Unwrap()

	// 4. FASE 2: Dekripsi Kunci AES
	fmt.Println("[2/4] Mendekripsi Payload AES-256-CBC...")
	decryptRes := aesDecrypter.DecryptURL(metadata.EncryptedEmbedURL(), metadata.DecryptionKey())
	if decryptRes.IsErr() {
		_, err := decryptRes.Unwrap()
		fmt.Printf("Fase 2 Gagal: %v\n", err)
		os.Exit(1)
	}
	iframeURL, _ := decryptRes.Unwrap()

	// 5. FASE 3: Menembus API JeniusPlay untuk mendapatkan M3U8
	fmt.Println("[3/4] Mengekstrak Master M3U8 dari JeniusPlay...")
	masterM3U8, err := extractJeniusPlayM3U8(iframeURL, targetURL)
	if err != nil {
		fmt.Printf("Fase 3 Gagal: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("      -> Master Playlist ditemukan!\n")

	// 6. FASE 4: Pengunduhan Konkuren (Goroutines)
	fmt.Printf("[4/4] Memulai Pengunduhan Paralel dengan %d Pekerja...\n", *workersPtr)
	start := time.Now()
	err = hlsDownloader.DownloadVideo(masterM3U8, outputFile)
	if err != nil {
		fmt.Printf("\n Fase 4 Gagal: %v\n", err)
		os.Exit(1)
	}

	duration := time.Since(start)
	fmt.Println("==================================================")
	fmt.Printf("SUKSES! Video berhasil disimpan ke: %s\n", outputFile)
	fmt.Printf("Waktu Pengunduhan: %s\n", duration.Round(time.Second))
	fmt.Println("==================================================")
}

// extractJeniusPlayM3U8 adalah logika API JeniusPlay yang kita pindahkan dari file test
func extractJeniusPlayM3U8(iframeURL, refererURL string) (string, error) {
	parts := strings.Split(strings.TrimRight(iframeURL, "/"), "/")
	hash := parts[len(parts)-1]

	jeniusAPI := "https://jeniusplay.com/player/index.php?data=" + hash + "&do=getVideo"
	payload := "hash=" + hash + "&r=" + refererURL

	req, err := http.NewRequest(http.MethodPost, jeniusAPI, strings.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("gagal membuat request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Referer", iframeURL)
	req.Header.Set("Origin", "https://jeniusplay.com")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("network error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("API menolak dengan status HTTP %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)

	videoSourceRegex := regexp.MustCompile(`"videoSource"\s*:\s*"([^"]+)"`)
	match := videoSourceRegex.FindStringSubmatch(string(body))

	if len(match) < 2 {
		return "", fmt.Errorf("videoSource tidak ditemukan dalam response JSON API")
	}

	masterM3U8 := strings.ReplaceAll(match[1], `\/`, `/`)
	if lastDot := strings.LastIndex(masterM3U8, "."); lastDot != -1 {
		masterM3U8 = masterM3U8[:lastDot] + ".m3u8"
	}

	return masterM3U8, nil
}
