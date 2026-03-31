package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/imamputra1/idlix-downloader/internal/infrastructure/crypto"
	"github.com/imamputra1/idlix-downloader/internal/infrastructure/downloader"
	"github.com/imamputra1/idlix-downloader/internal/infrastructure/scraper"
)

type SubtitleTrack struct {
	Label  string
	URL    string
	Format string
	SizeKB float64
}

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
	previewPtr := flag.Bool("preview", false, "Hanya tampilkan resolusi dan Subtitle yang tersedia tanpa mengunduh")
	subPtr := flag.String("sub", "all", "Opsi Subtitle: 'all', 'none', atau 'atau Label spesifik'")
	flag.Parse()

	if *urlPtr == "" {
		fmt.Println(" Error: URL wajib diisi!")
		os.Exit(1)
	}

	fmt.Println("==================================================")
	fmt.Println("IDLIX DOWNLOADER ENGINE - V1.0 (Pratinjau)")
	fmt.Println("==================================================")

	domScraper := scraper.NewNativeHTTPScraper()
	aesDecrypter := crypto.NewAESDecrypter()
	hlsDownloader := downloader.NewHLSDownloader(*workersPtr)

	fmt.Printf("[1/6] Menembus pertahanan WAF dan mengekstrak Payload...\n")
	scrapeRes := domScraper.ScraperMetadata(*urlPtr)
	if scrapeRes.IsErr() {
		os.Exit(1)
	}
	metadata, _ := scrapeRes.Unwrap()

	fmt.Println("[2/6] Mendekripsi Payload AES-256-CBC...")
	decryptRes := aesDecrypter.DecryptURL(metadata.EncryptedEmbedURL(), metadata.DecryptionKey())
	if decryptRes.IsErr() {
		os.Exit(1)
	}
	iframeURL, _ := decryptRes.Unwrap()

	fmt.Printf("    -> Link Sumber %s\n", iframeURL)

	fmt.Println("[3/6] Mengekstrak Master M3U8 dan Inspeksi Subtitle...")
	masterM3U8, subtitles, err := extractJeniusPlayM3U8(iframeURL, *urlPtr)
	if err != nil {
		fmt.Printf("\n fase 3 Gagal : %v", err)
		os.Exit(1)
	}

	fmt.Printf("[4/6] Mengambil Katalog Resolusi dan Menghitung Estimasi Ukuran...")
	resolutions, err := hlsDownloader.ExtractResolutions(masterM3U8)
	if err != nil {
		fmt.Printf("\n fase 4 Gagal: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\n [Katalog Video]")
	var selectedRes *downloader.VideoResolution
	highestBw := -1

	for _, res := range resolutions {
		sizeInfo := "Tidak diketahui"
		if res.EstimatedSizeMB > 0 {
			sizeInfo = fmt.Sprintf("~%.2f MB", res.EstimatedSizeMB)
		}

		marker := "  "
		if (*resPtr == "best" && res.Bandwidth > highestBw) || res.Resolution == *resPtr {
			marker = "->"
			if *resPtr == "best" {
				highestBw = res.Bandwidth
			}
			resCopy := res
			selectedRes = &resCopy
		}

		fmt.Printf(" %s - Resolusi: %-10s | %-12s | Bandwidth: %d bps\n", marker, res.Resolution, sizeInfo, res.Bandwidth)
	}

	fmt.Println("\n [Katalog Subtitle]")
	if len(subtitles) == 0 {
		fmt.Println("   - (Kosong) Server tidak menyediakan file teks.")
	} else {
		for _, sub := range subtitles {
			sizeInfo := "N/A"
			if sub.SizeKB > 0 {
				sizeInfo = fmt.Sprintf("%.2f KB", sub.SizeKB)
			}
			marker := "  "
			if *subPtr == "all" || strings.EqualFold(sub.Label, *subPtr) {
				marker = "->"
			} else if *subPtr == "none" {
				marker = "[X]"
			}
			fmt.Printf(" %s - Label: %-10s | Format: %-4s | Ukuran: %s\n", marker, sub.Label, strings.ToUpper(sub.Format), sizeInfo)
		}
	}

	if *previewPtr {
		fmt.Printf("\n Mode Pratinjau selesai. Anda dapat meninjau ukuran sebelum mengeksekusi I/O disk.")
		os.Exit(0)
	}

	if selectedRes == nil {
		fmt.Printf("\n Resolusi Target '%s' tidak ditemukan di Katalog.\n", *resPtr)
		os.Exit(1)
	}

	finalOutputDir := expandPath(*dirPtr)
	os.MkdirAll(finalOutputDir, 0o755)
	finalOutputPath := filepath.Join(finalOutputDir, *namePtr)

	fmt.Printf("\n[5/6] Mengeksekusi Video [%s] ke Disk ...", selectedRes.Resolution)
	start := time.Now()
	err = hlsDownloader.ExecuteDownload(selectedRes.PlaylistURL, finalOutputPath)
	if err != nil {
		fmt.Printf("\n Gagal: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("\n[6/6] Memproses Subtitle (Kebijakan: %s)...\n", *subPtr)
	if *subPtr != "none" && len(subtitles) > 0 {
		for _, sub := range subtitles {
			if *subPtr == "all" || strings.EqualFold(sub.Label, *subPtr) {
				downloadSubtitle(sub, finalOutputPath)
			}
		}
	} else if *subPtr == "none" {
		fmt.Println("    -> Dilewati sesuai instruksi pengguna")
	}

	duration := time.Since(start)
	fmt.Println("\n==================================================")
	fmt.Printf(" OPERASI SELESAI!\n Tersimpan di: %s\n", finalOutputPath)
	fmt.Printf(" Waktu Eksekusi: %s\n", duration.Round(time.Second))
	fmt.Println("==================================================")
}

func extractJeniusPlayM3U8(iframeURL, refererURL string) (string, []SubtitleTrack, error) {
	cleanIframeURL := strings.TrimSpace(iframeURL)
	cleanIframeURL = strings.ReplaceAll(cleanIframeURL, "\n", "")
	cleanIframeURL = strings.ReplaceAll(cleanIframeURL, "\r", "")
	cleanIframeURL = strings.ReplaceAll(cleanIframeURL, "\\", "")

	client := &http.Client{Timeout: 15 * time.Second}

	var hash string
	var htmlStr string

	reqHtml, _ := http.NewRequest("GET", cleanIframeURL, nil)
	reqHtml.Header.Set("User-Agent", "Mozilla/5.0 (Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")
	reqHtml.Header.Set("Referer", refererURL)

	if respHtml, err := client.Do(reqHtml); err == nil && respHtml.StatusCode == 200 {
		htmlBytes, _ := io.ReadAll(respHtml.Body)
		htmlStr = string(htmlBytes)
		respHtml.Body.Close()
	}

	if strings.Contains(cleanIframeURL, "data=") {
		parts := strings.Split(cleanIframeURL, "data=")
		if len(parts) > 1 {
			hash = parts[1]
			if strings.Contains(hash, "&") {
				hash = strings.Split(hash, "&")[0]
			}
		}
	} else {
		parsedURL, err := url.Parse(iframeURL)
		if err == nil {
			parts := strings.Split(strings.TrimRight(parsedURL.Path, "/"), "/")
			hash = parts[len(parts)-1]
		}
	}

	if hash == "" || hash == "index.php" {
		return "", nil, fmt.Errorf("Gagal mengekstrak ID hash dari URL iframeURL: %s", cleanIframeURL)
	}

	jeniusAPI := "https://jeniusplay.com/player/index.php?data=" + hash + "&do=getVideo"
	payload := "hash=" + hash + "&r=" + refererURL

	req, _ := http.NewRequest(http.MethodPost, jeniusAPI, strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Referer", cleanIframeURL)
	req.Header.Set("Origin", "https://jeniusplay.com")
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("network error ke API JeniusPlay: %w", err)
	}
	if resp.StatusCode != 200 {
		return "", nil, fmt.Errorf("API menolak dengan status HTTP %d", resp.StatusCode)
	}
	defer resp.Body.Close()

	fmt.Printf("   -> Header Server: ContentLength = %v, Content-Type = %v\n", resp.Header.Get("Content-Length"), resp.Header.Get("Content-Type"))

	body, _ := io.ReadAll(resp.Body)
	if len(body) == 0 {
		return "", nil, fmt.Errorf("server memberikan silent drop (body kosong). WAF kemungkinan memblokir finger print bot")
	}
	bodyStr := string(body)

	var masterM3U8 string
	videoSourceMatch := regexp.MustCompile(`"videoSource"\s*:\s*"([^"]+)"`).FindStringSubmatch(bodyStr)

	if len(videoSourceMatch) > 1 {
		masterM3U8 = videoSourceMatch[1]
	} else {
		fallbackMatch := regexp.MustCompile(`"file"\s*:\s*"([^"]+\.m3u8)"`).FindStringSubmatch(bodyStr)
		if len(fallbackMatch) > 1 {
			masterM3U8 = fallbackMatch[1]
		} else {
			debugFilename := "debug_error_fase3.html"
			errDump := os.WriteFile(debugFilename, body, 0o644)
			if errDump != nil {
				return "", nil, fmt.Errorf("URL m3u8 tidak ditemukan dan gagal menulis debug", errDump)
			}
			return "", nil, fmt.Errorf("URL M3U8 tidak ditemukan. Respons mentah server telah disalin ke %s", debugFilename)
			// return "", nil, fmt.Errorf("URL m3u8 tidak ditemukan. Respons Server: %s...", bodyStr[:min(100, len(bodyStr))])
		}
	}

	masterM3U8 = strings.ReplaceAll(masterM3U8, `\/`, `/`)
	if lastDot := strings.LastIndex(masterM3U8, "."); lastDot != -1 {
		masterM3U8 = masterM3U8[:lastDot] + ".m3u8"
	}

	var subtitles []SubtitleTrack

	combinedDatastr := htmlStr + "\n" + bodyStr

	objectChunk := strings.Split(combinedDatastr, "}")
	fileRegex := regexp.MustCompile(`(?i)(?:"|')?file(?:"|')?\s*:\s*(?:"|')([^"']+)(?:"|')`)
	labelRegex := regexp.MustCompile(`(?i)(?:"|')?label(?:"|')?\s*:\s*(?:"|')([^"']+)(?:"|')`)

	for _, chunk := range objectChunk {
		fileMatch := fileRegex.FindStringSubmatch(chunk)

		if len(fileMatch) > 1 {
			subURL := strings.ReplaceAll(fileMatch[1], `\/`, `/`)
			lowerUrl := strings.ToLower(subURL)
			if !strings.Contains(lowerUrl, ".vtt") && !strings.Contains(lowerUrl, ".txt") && !strings.Contains(lowerUrl, ".srt") {
				continue
			}

			subLabel := "Unknown"
			labelMatch := labelRegex.FindStringSubmatch(chunk)
			if len(labelMatch) > 1 {
				subLabel = labelMatch[1]
			}

			ext := "vtt"
			if strings.Contains(lowerUrl, ".srt") {
				ext = "srt"
			} else if strings.Contains(lowerUrl, ".txt") {
				ext = "txt"
			}
			var sizeKB float64

			headReq, _ := http.NewRequest("HEAD", subURL, nil)
			headReq.Header.Set("User-Agent", "Mozilla/5.0")
			if headResp, err := client.Do(headReq); err == nil {
				if headResp.ContentLength > 0 {
					sizeKB = float64(headResp.ContentLength) / 1024.0
				}
				headResp.Body.Close()
			}

			isDuplicate := false
			for _, existingSub := range subtitles {
				if existingSub.URL == subURL {
					isDuplicate = true
					break
				}
			}

			if !isDuplicate {
				subtitles = append(subtitles, SubtitleTrack{
					URL:    subURL,
					Label:  subLabel,
					Format: strings.TrimPrefix(ext, "."),
					SizeKB: sizeKB,
				})
			}
		}
	}

	if len(subtitles) > 0 {
		rawURLRegex := regexp.MustCompile(`(?i)https?:\/\/[^"'\s]+\.(?:vtt|srt|txt)`)
		allURLs := rawURLRegex.FindAllString((combinedDatastr), -1)

		for i, subURL := range allURLs {
			cleanURL := strings.ReplaceAll(subURL, `\/`, `/`)
			isDuplicate := false
			for _, existingSub := range subtitles {
				if existingSub.URL == cleanURL {
					isDuplicate = true
					break
				}
			}
			if !isDuplicate {
				subtitles = append(subtitles, SubtitleTrack{
					URL:    cleanURL,
					Label:  fmt.Sprint("Track_%", i+1),
					Format: "Vtt",
					SizeKB: 0,
				})
			}
		}
	}

	return masterM3U8, subtitles, nil
}

func downloadSubtitle(sub SubtitleTrack, videoFilename string) {
	fmt.Printf("   -> %s...", sub.Label)
	req, _ := http.NewRequest("GET", sub.URL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != 200 {
		fmt.Printf("Gagal HTTP\n")
		return
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	ext := "." + sub.Format
	if ext == "." {
		ext = ".vtt"
	}
	baseName := strings.TrimSuffix(videoFilename, filepath.Ext(videoFilename))
	cleanLabel := strings.ReplaceAll(sub.Label, " ", "_")
	subPath := fmt.Sprintf("%s_%s%s", baseName, cleanLabel, ext)

	if err := os.WriteFile(subPath, data, 0o644); err == nil {
		fmt.Printf("%s\n", filepath.Base(subPath))
	} else {
		fmt.Printf("Gagal i/O\n")
	}
}
