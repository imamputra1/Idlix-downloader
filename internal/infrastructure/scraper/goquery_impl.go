package scraper

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
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
	// 1. Jalankan Chrome tanpa headless agar bisa melewati Cloudflare dengan mudah
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("mute-audio", true),
	)

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelAlloc()

	ctx, cancelCtx := chromedp.NewContext(allocCtx)
	defer cancelCtx()

	// Beri waktu lebih lama karena memuat video butuh proses
	ctx, cancelTimeout := context.WithTimeout(ctx, 45*time.Second)
	defer cancelTimeout()

	// Channel untuk menangkap URL m3u8
	m3u8Found := make(chan string, 1)

	// 2. PASANG RADAR JARINGAN (Hanya mencari ekstensi file video)
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		switch e := ev.(type) {
		case *network.EventRequestWillBeSent:
			url := e.Request.URL
			// Jika radar mendeteksi request ke file .m3u8 (HLS Playlist)
			if strings.Contains(url, ".m3u8") && !strings.Contains(url, "ad") {
				select {
				case m3u8Found <- url:
				default:
				}
			}
		}
	})

	fmt.Printf("\n[SNIFFER] Menunggu browser memutar video dan mengekstrak M3U8 di: %s\n", targetURL)

	// 3. JALANKAN BROWSER & KLIK TOMBOL PLAY PAKSA
	err := chromedp.Run(ctx,
		network.Enable(),
		chromedp.Navigate(targetURL),
		chromedp.Sleep(8*time.Second), // Tunggu Cloudflare / Loading awal selesai

		// Injeksi JS untuk membasmi iklan dan memaksa tombol play ter-klik
		chromedp.EvaluateAsDevTools(`
			(function() {
				// Hapus semua iklan overlay yang menutupi layar
				let overlays = document.querySelectorAll('.ad-overlay, .pop-up, [style*="z-index: 999"]');
				overlays.forEach(el => el.remove());

				// Paksa klik semua tombol yang berpotensi memutar video
				const selectors = ['#play-iframe', '#player-option-1', '.play-video', '.mvic-play', 'iframe'];
				for (let sel of selectors) {
					let elements = document.querySelectorAll(sel);
					elements.forEach(el => el.click());
				}
				
				// Klik brutal di tengah layar (fallback)
				let centerX = window.innerWidth / 2;
				let centerY = window.innerHeight / 2;
				let el = document.elementFromPoint(centerX, centerY);
				if(el) el.click();
			})();
		`, nil),
	)
	if err != nil {
		return result.Err[entities.VideoMetadata](fmt.Errorf("gagal menavigasi: %w", err))
	}

	// 4. MENUNGGU HASIL TANGKAPAN RADAR
	select {
	case finalURL := <-m3u8Found:
		fmt.Printf("[SNIFFER] JACKPOT! Master M3U8 berhasil disadap: %s\n", finalURL)

		// Rakit entity langsung dengan hasil akhirnya (Tidak perlu AES lagi!)
		metadata := entities.NewVideoMetadata("sniffed_id", "Sniffed Video", finalURL, "no-key-needed", finalURL)
		return result.Ok(metadata)

	case <-ctx.Done():
		return result.Err[entities.VideoMetadata](errors.New("timeout: Video tidak pernah berputar atau M3U8 tidak ditemukan"))
	}
}
