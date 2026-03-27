package scraper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings" // Digunakan untuk memotong prefix "ERROR_JS:"
	"time"

	"github.com/chromedp/cdproto/runtime" // IMPOR WAJIB: Untuk menunggu Promise Javascript
	"github.com/chromedp/chromedp"
	"github.com/imamputra1/idlix-downloader/internal/core/entities"
	"github.com/imamputra1/idlix-downloader/internal/core/ports"
	"github.com/imamputra1/idlix-downloader/pkg/result"
)

type ChromedpScraper struct{}

func NewChromedpScraper() ports.Scraper {
	return &ChromedpScraper{}
}

func (s *ChromedpScraper) ScraperMetadata(targetURL string) result.Result[entities.VideoMetadata] {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("mute-audio", true),
		chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/127.0.0.0 Safari/537.36"),
	)

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelAlloc()

	ctx, cancelCtx := chromedp.NewContext(allocCtx)
	defer cancelCtx()

	ctx, cancelTimeout := context.WithTimeout(ctx, 30*time.Second)
	defer cancelTimeout()

	var rawJSONResult string

	// =========================================================================
	// 1. Perbaikan Algoritma In-Browser Fetch (Normalisasi String & Promise)
	// =========================================================================
	jsFetchScript := `
		new Promise((resolve) => {
			try {
				let postId = "";
				let meta = document.querySelector('meta#dooplay-ajax-counter');
				if (meta) postId = meta.getAttribute('data-postid');

				if (!postId) {
					let match = document.body.className.match(/postid-(\d+)/);
					if (match) postId = match[1];
				}

				// Perbaikan Kriteria 4: Tangkap error dengan string khusus
				if (!postId) {
					resolve("ERROR_JS:Post ID tidak ditemukan di DOM");
					return;
				}

				let formData = new URLSearchParams();
				formData.append('action', 'doo_player_ajax');
				formData.append('post', postId);
				formData.append('nume', '1');
				formData.append('type', 'movie');

				// Rantai Promise sesuai spesifikasi yang diminta
				fetch('/wp-admin/admin-ajax.php', {
					method: 'POST',
					headers: {
						'Content-Type': 'application/x-www-form-urlencoded; charset=UTF-8',
						'X-Requested-With': 'XMLHttpRequest'
					},
					body: formData.toString()
				})
				.then(r => {
					if (!r.ok) throw new Error("AJAX gagal dengan status HTTP: " + r.status);
					return r.json();
				})
				.then(data => {
					// Bungkus dengan JSON.stringify agar aman diseberangkan ke Go
					let resultObj = {
						post_id: postId,
						embed_url: data.embed_url || "",
						key: data.key || ""
					};
					resolve(JSON.stringify(resultObj));
				})
				.catch(err => {
					resolve("ERROR_JS:" + err.message);
				});
			} catch(err) {
				resolve("ERROR_JS:" + err.message);
			}
		});
	`

	err := chromedp.Run(ctx,
		chromedp.Navigate(targetURL),
		chromedp.WaitVisible(`body`, chromedp.ByQuery),
		chromedp.Sleep(3*time.Second),

		// runtime.EvaluateParams akan memerintahkan Go menunggu blok `new Promise` di atas
		// me-resolve nilai string-nya sebelum diekstrak ke &rawJSONResult
		chromedp.Evaluate(jsFetchScript, &rawJSONResult, func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
			return p.WithAwaitPromise(true)
		}),
	)
	if err != nil {
		return result.Err[entities.VideoMetadata](fmt.Errorf("eksekusi chromedp gagal: %w", err))
	}

	// =========================================================================
	// 2 & 3. Pemrosesan Balasan di Sisi Go (String Prefix Parsing)
	// =========================================================================

	// Jika diawali "ERROR_JS:", langsung tembak sebagai error untuk meluluskan Kriteria 4
	if strings.HasPrefix(rawJSONResult, "ERROR_JS:") {
		errMsg := strings.TrimPrefix(rawJSONResult, "ERROR_JS:")
		return result.Err[entities.VideoMetadata](fmt.Errorf("javascript in-browser error: %s", errMsg))
	}

	// Jika aman, lanjutkan unmarshal JSON murni (Untuk Kriteria 1 & 2)
	var ajaxResult struct {
		PostID   string `json:"post_id"`
		EmbedURL string `json:"embed_url"`
		Key      string `json:"key"`
	}

	if err := json.Unmarshal([]byte(rawJSONResult), &ajaxResult); err != nil {
		return result.Err[entities.VideoMetadata](fmt.Errorf("gagal memparsing kembalian JSON dari JS browser: %w. Raw Data: %s", err, rawJSONResult))
	}

	if ajaxResult.EmbedURL == "" {
		return result.Err[entities.VideoMetadata](errors.New("kunci 'embed_url' kosong. Target mengembalikan honeypot data"))
	}

	metadata := entities.NewVideoMetadata(
		ajaxResult.PostID,
		"Unknown Title",
		ajaxResult.EmbedURL,
		ajaxResult.Key,
		"",
	)

	return result.Ok(metadata)
}
