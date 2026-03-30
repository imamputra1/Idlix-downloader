package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/imamputra1/idlix-downloader/internal/infrastructure/crypto"
	"github.com/imamputra1/idlix-downloader/internal/infrastructure/downloader"
	"github.com/imamputra1/idlix-downloader/internal/infrastructure/scraper"
)

func expandPath(pathStr string) string {
	if strings.HasPrefix(pathStr, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, pathStr[1:])
		}
	}
	return filepath.Clean(pathStr)
}

func main() {
	urlPtr := flag.String("url", "", "URL Film atau Series dari Idlix")
	dirPtr := flag.String("dir", "~/Downloads/Idlix", "Direktori penyimpanan (contoh: ~/Downloads/Idlix)")
	namePtr := flag.String("name", "movie.mp4", "Nama file hasil unduhan (contoh: mr_robot_s3.mp4)")
	workersPtr := flag.Int("workers", 15, "Jumlah Goroutine paralel")
	resPtr := flag.String("res", "best", "Target resolusi (contoh: 1920x1080, 1280x720, best)")
	previewPtr := flag.Bool("preview", false, "Hanya tampilkan resolusi yang tersedia tanpa mengunduh")
	flag.Parse()

	if *urlPtr == "" {
		fmt.Println(" Error: URL wajib diisi!")
		os.Exit(1)
	}

	fmt.Println("==================================================")
	fmt.Println("IDLIX DOWNLOADER ENGINE - V1.0 (Decoupled & API Ready)")
	fmt.Println("==================================================")

	domScraper := scraper.NewNativeHTTPScraper()
	aesDecrypter := crypto.NewAESDecrypter()
	hlsDownloader := downloader.NewHLSDownloader(*workersPtr)

	fmt.Printf("[1/5] Menembus pertahanan WAF dan mengekstrak Payload...\n")
	scrapeRes := domScraper.ScraperMetadata(*urlPtr)
	if scrapeRes.IsErr() {
		os.Exit(1)
	}
	metadata, _ := scrapeRes.Unwrap()

	fmt.Println("[2/5] Mendekripsi Payload AES-256-CBC...")
	decryptRes := aesDecrypter.DecryptURL(metadata.EncryptedEmbedURL(), metadata.DecryptionKey())
	if decryptRes.IsErr() {
		os.Exit(1)
	}
	iframeURL, _ := decryptRes.Unwrap()

	fmt.Println("[3/5] Mengekstrak Master M3U8 dari JeniusPlay...")
	masterM3U8, err := extractJeniusPlayM3U8(iframeURL, *urlPtr)
	if err != nil {
		fmt.Printf(" Fase 3 Gagal: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("[4/5] Mengambil Katalog Resolusi...")
	resolutions, err := hlsDownloader.ExtractResolutions(masterM3U8)
	if err != nil {
		fmt.Printf("Fase 4 Gagal: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\n Resolusi Tersedia:")
	var selectedRes *downloader.VideoResolution
	highestBw := -1

	for _, res := range resolutions {
		fmt.Printf("   - Resolusi: %-10s | Bandwidth: %d bps\n", res.Resolution, res.Bandwidth)

		if *resPtr == "best" {
			if res.Bandwidth > highestBw {
				highestBw = res.Bandwidth
				selectedRes = &res
			}
		} else if res.Resolution == *resPtr {
			selectedRes = &res
		}
	}

	if *previewPtr {
		fmt.Printf("\n Mode preview selesai. Gunakan flag -res=\"resolusi\" untuk mendownload")
		os.Exit(0)
	}

	if selectedRes == nil {
		fmt.Printf("\n Resolusi '%s' tidak ditemukan di dalam Katalog. \n", *resPtr)
		os.Exit(1)
	}
	fmt.Printf("\n Menetapkan Target eksekusi: %s (%d bps)\n", selectedRes.Resolution, selectedRes.Bandwidth)

	finalOutputDir := expandPath(*dirPtr)

	if err := os.MkdirAll(finalOutputDir, 0o755); err != nil {
		fmt.Printf("Gagal membuat Direktori: %v\n", err)
		os.Exit(1)
	}

	finalOutputPath := filepath.Join(finalOutputDir, *namePtr)

	fmt.Printf("[5/5] Mengeksekusi Pengunduhan \n")
	start := time.Now()

	err = hlsDownloader.ExecuteDownload(selectedRes.PlaylistURL, finalOutputPath)
	if err != nil {
		fmt.Printf("\n Fase 5 Gagal: %v\n", err)
		os.Exit(1)
	}

	duration := time.Since(start)
	fmt.Println("==================================================")
	fmt.Printf(" SUKSES! Video berhasil dirakit dan disimpan ke: %s\n", finalOutputPath)
	fmt.Printf(" Waktu Pengunduhan: %s\n", duration.Round(time.Second))
	fmt.Println("==================================================")
}

func extractJeniusPlayM3U8(iframeURL, refererURL string) (string, error) {
	parts := strings.Split(strings.TrimRight(iframeURL, "/"), "/")
	hash := parts[len(parts)-1]
	jeniusAPI := "https://jeniusplay.com/player/index.php?data=" + hash + "&do=getVideo"
	payload := "hash=" + hash + "&r=" + refererURL

	req, _ := http.NewRequest(http.MethodPost, jeniusAPI, strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Referer", iframeURL)
	req.Header.Set("Origin", "https://jeniusplay.com")
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64)")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != 200 {
		return "", fmt.Errorf("API error atau ditolak")
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	match := regexp.MustCompile(`"videoSource"\s*:\s*"([^"]+)"`).FindStringSubmatch(string(body))
	if len(match) < 2 {
		return "", fmt.Errorf("videoSource tidak ditemukan")
	}

	masterM3U8 := strings.ReplaceAll(match[1], `\/`, `/`)
	if lastDot := strings.LastIndex(masterM3U8, "."); lastDot != -1 {
		masterM3U8 = masterM3U8[:lastDot] + ".m3u8"
	}
	return masterM3U8, nil
}
