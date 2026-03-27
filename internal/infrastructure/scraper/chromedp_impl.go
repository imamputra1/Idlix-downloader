package scraper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/imamputra1/idlix-downloader/internal/core/entities"
	"github.com/imamputra1/idlix-downloader/internal/core/ports"
	"github.com/imamputra1/idlix-downloader/pkg/result"
)

type ChromedpScraper struct{}

// NewChromedpScraper menginisiasi scraper berbasis peramban tanpa antarmuka
func NewChromedpScraper() ports.Scraper {
	return &ChromedpScraper{}
}

func (s *ChromedpScraper) ScraperMetadata(targetURL string) result.Result[entities.VideoMetadata] {
	// 1. Konfigurasi Penyamaran (Stealth Flags)
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("excludeSwitches", "enable-automation"),
		chromedp.Flag("use-mock-keychain", "true"),
		chromedp.Flag("mute-audio", true),
		// Memblokir notifikasi dan pop-up di tingkat peramban
		// chromedp.Flag("disable-popup-blocking", false),
		// chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/127.0.0.0 Safari/537.36"),
		chromedp.UserDataDir("/tmp/idlix_bodoh_profile"),
	)

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelAlloc()

	ctx, cancelCtx := chromedp.NewContext(allocCtx)
	defer cancelCtx()

	// Batas waktu eksekusi total
	ctx, cancelTimeout := context.WithTimeout(ctx, 60*time.Second)
	defer cancelTimeout()

	var targetRequestID network.RequestID
	var responseBody []byte
	ajaxFound := make(chan network.RequestID, 1)

	var htmlDump string
	var screenshotBuf []byte

	// 2. Setup Radar Jaringan
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		switch e := ev.(type) {
		case *network.EventResponseReceived:
			if strings.Contains(e.Response.URL, "/wp-admin/admin-ajax.php") && e.Response.Status == 200 {
				targetRequestID = e.RequestID
			}

		case *network.EventLoadingFinished:
			if targetRequestID != "" && e.RequestID == targetRequestID {
				select {
				case ajaxFound <- e.RequestID:
				default:
				}
			}
		}
	})

	fmt.Printf("\n[CHROMEDP] Menggelar Radar Jaringan ke: %s\n", targetURL)

	// Skrip Injeksi JavaScript Presisi (Surgical Strike)
	jsSurgicalStrike := `
		(function() {
			try {
				// 1. NEUTRALISASI REDIRECT & POP-UNDER
				// Melumpuhkan kemampuan situs untuk melempar tab kita ke situs iklan
				window.open = function() { console.log("Dicegah: window.open"); return null; };
				
				// 2. SURGICAL CLICK (Klik Presisi)
				// Di arsitektur DooPlay, mengklik daftar server atau tombol play utama akan langsung memicu AJAX
				let serverTab = document.querySelector('ul#playeroptionsul li');
				if (serverTab) {
					serverTab.click();
					return "SUKSES: Tab Server DooPlay diklik";
				}

				let playMain = document.querySelector('#play-iframe, .play-video, .mvic-play, #btn-play');
				if (playMain) {
					playMain.click();
					return "SUKSES: Tombol Play Utama diklik";
				}

				return "GAGAL: Tidak menemukan pemicu DOM standar";
			} catch (e) {
				return "ERROR JS: " + e.message;
			}
		})();
	`

	var jsExecutionResult string

	// 3. Eksekusi Urutan Peramban
	err := chromedp.Run(ctx,
		network.Enable(),

		chromedp.Navigate(targetURL),

		// Berikan waktu yang cukup agar Cloudflare Turnstile memberikan Clearance Cookie (8 detik sangat direkomendasikan)
		chromedp.Sleep(8*time.Second),

		chromedp.OuterHTML("html", &htmlDump),
		chromedp.FullScreenshot(&screenshotBuf, 90),

		// Eksekusi klik presisi
		chromedp.Evaluate(jsSurgicalStrike, &jsExecutionResult),
	)
	if err != nil {
		return result.Err[entities.VideoMetadata](fmt.Errorf("gagal menavigasi atau mengeksekusi pemicu: %w", err))
	}

	os.WriteFile("debug_page.html", []byte(htmlDump), 0o644)
	os.WriteFile("debug_screenshot.png", screenshotBuf, 0o644)
	fmt.Printf("[CHROMEDP] Bukti forensik disimpan: debug_page.html dan debug_screenshot.png\n")

	fmt.Printf("[CHROMEDP] Status Injeksi JS: %s\n", jsExecutionResult)

	// 4. Menunggu Sinyal Jaringan
	select {
	case reqID := <-ajaxFound:
		fmt.Printf("[CHROMEDP] Target AJAX terdeteksi (ReqID: %s)! Mengekstrak payload...\n", reqID)

		err := chromedp.Run(ctx, chromedp.ActionFunc(func(c context.Context) error {
			var err error
			responseBody, err = network.GetResponseBody(reqID).Do(c)
			return err
		}))
		if err != nil {
			return result.Err[entities.VideoMetadata](fmt.Errorf("gagal mengekstrak body dari request: %w", err))
		}

	case <-ctx.Done():
		return result.Err[entities.VideoMetadata](errors.New("timeout: target AJAX tidak terpicu. Kemungkinan struktur DOM berubah tajam atau Cloudflare memblokir total"))
	}

	// 5. Ekstraksi JSON Kriptografi
	var ajaxResult struct {
		EmbedURL string `json:"embed_url"`
		Key      string `json:"key"`
	}

	if err := json.Unmarshal(responseBody, &ajaxResult); err != nil {
		ajaxResult.EmbedURL = strings.TrimSpace(string(responseBody))
	}

	if ajaxResult.EmbedURL == "" {
		return result.Err[entities.VideoMetadata](fmt.Errorf("body jaringan berhasil diekstrak tapi embed_url kosong: %s", string(responseBody)))
	}

	fmt.Printf("[CHROMEDP] Payload Kriptografi sukses dirampas! Panjang CT: %d\n\n", len(ajaxResult.EmbedURL))

	// Mengemas entitas untuk didekripsi oleh lapisan kriptografi Anda
	metadata := entities.NewVideoMetadata(
		"intercepted_via_network",
		"Unknown Title",
		ajaxResult.EmbedURL,
		ajaxResult.Key,
		"",
	)

	return result.Ok(metadata)
}
