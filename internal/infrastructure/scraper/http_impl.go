package scraper

import (
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"

	http "github.com/bogdanfinn/fhttp" // WAJIB: Mengganti net/http bawaan Go
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
	"github.com/imamputra1/idlix-downloader/internal/core/entities"
	"github.com/imamputra1/idlix-downloader/internal/core/ports"
	"github.com/imamputra1/idlix-downloader/pkg/result"
)

type NativeHTTPScraper struct {
	client tls_client.HttpClient
}

// NewNativeHTTPScraper menginisiasi HTTP Client tahan banting dengan TLS Client
func NewNativeHTTPScraper() ports.Scraper {
	jar := tls_client.NewCookieJar()
	options := []tls_client.HttpClientOption{
		tls_client.WithTimeoutSeconds(30),
		tls_client.WithClientProfile(profiles.Chrome_120),
		tls_client.WithCookieJar(jar), // Menyimpan session clearance otomatis
		// tls_client.WithNotFollowRedirects(),
	}

	// Inisialisasi klien yang sidik jarinya identik dengan Chrome
	client, err := tls_client.NewHttpClient(tls_client.NewNoopLogger(), options...)
	if err != nil {
		fmt.Printf("Gagal inisialisasi TLS Client: %v\n", err)
	}

	return &NativeHTTPScraper{
		client: client,
	}
}

func (s *NativeHTTPScraper) ScraperMetadata(targetURL string) result.Result[entities.VideoMetadata] {
	// ==========================================================
	// FASE 1: GET Halaman Utama
	// ==========================================================
	req, err := http.NewRequest(http.MethodGet, targetURL, nil)
	if err != nil {
		return result.Err[entities.VideoMetadata](fmt.Errorf("gagal membuat request: %w", err))
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := s.client.Do(req)
	if err != nil {
		return result.Err[entities.VideoMetadata](fmt.Errorf("network error Fase 1: %w", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return result.Err[entities.VideoMetadata](fmt.Errorf("fase 1 ditolak server, Status: %d", resp.StatusCode))
	}

	htmlBytes, _ := io.ReadAll(resp.Body)
	htmlContent := string(htmlBytes)

	// Ekstraksi ID
	postIDRegex := regexp.MustCompile(`p=(\d+)"|data-postid="(\d+)"|postid-(\d+)`)
	postIDMatch := postIDRegex.FindStringSubmatch(htmlContent)
	var postID string
	for _, match := range postIDMatch[1:] {
		if match != "" {
			postID = match
			break
		}
	}
	if postID == "" {
		return result.Err[entities.VideoMetadata](fmt.Errorf("Post ID tidak ditemukan"))
	}

	// Ekstraksi Nonce
	nonceRegex := regexp.MustCompile(`"nonce":"([^"]+)"|data-nonce="([^"]+)"`)
	nonceMatch := nonceRegex.FindStringSubmatch(htmlContent)
	var nonce string
	for _, match := range nonceMatch[1:] {
		if match != "" {
			nonce = match
			break
		}
	}

	fmt.Printf("[TLS-SCRAPER] Fase 1 Sukses! PostID: %s | Nonce: %s\n", postID, nonce)

	// ==========================================================
	// FASE 2: POST AJAX
	// ==========================================================
	parsedURL, _ := url.Parse(targetURL)
	ajaxURL := fmt.Sprintf("%s://%s/wp-admin/admin-ajax.php", parsedURL.Scheme, parsedURL.Host)

	formData := url.Values{}
	formData.Set("action", "doo_player_ajax")
	formData.Set("post", postID)
	formData.Set("nume", "1")
	formData.Set("type", "movie")
	if nonce != "" {
		formData.Set("nonce", nonce)
	}

	postReq, err := http.NewRequest(http.MethodPost, ajaxURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return result.Err[entities.VideoMetadata](fmt.Errorf("gagal membuat POST: %w", err))
	}

	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	postReq.Header.Set("X-Requested-With", "XMLHttpRequest")
	postReq.Header.Set("Referer", targetURL)
	postReq.Header.Set("Origin", fmt.Sprintf("%s://%s", parsedURL.Scheme, parsedURL.Host))
	postReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	postReq.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	postReq.Header.Set("Accept-Language", "en-US,en;q=0.9")

	postResp, err := s.client.Do(postReq)
	if err != nil {
		return result.Err[entities.VideoMetadata](fmt.Errorf("network error Fase 2: %w", err))
	}
	defer postResp.Body.Close()

	ajaxBytes, _ := io.ReadAll(postResp.Body)
	rawResponse := string(ajaxBytes)

	if postResp.StatusCode != 200 {
		return result.Err[entities.VideoMetadata](fmt.Errorf("server error %d: %s", postResp.StatusCode, rawResponse))
	}

	// Pengecekan Honeypot (0 bytes)
	if len(strings.TrimSpace(rawResponse)) == 0 {
		return result.Err[entities.VideoMetadata](fmt.Errorf("silent Drop (Body Kosong)"))
	}

	// ==========================================================
	// FASE 3: Parsing Custom
	// ==========================================================
	// Karena kita menggunakan fhttp, pastikan untuk menggunakan modul encoding standar
	// menggunakan trik manual jika json.Unmarshal gagal karena format non-standar

	// Gunakan regex presisi tinggi untuk merobek JSON
	embedRegex := regexp.MustCompile(`"embed_url"\s*:\s*"((?:\\.|[^"\\])*)"`)
	keyRegex := regexp.MustCompile(`"key"\s*:\s*"((?:\\.|[^"\\])*)"`)

	embedMatch := embedRegex.FindStringSubmatch(rawResponse)
	keyMatch := keyRegex.FindStringSubmatch(rawResponse)

	if len(embedMatch) < 2 {
		return result.Err[entities.VideoMetadata](fmt.Errorf("gagal menemukan embed_url di dalam raw response: %s", rawResponse))
	}

	rawEmbedURL := embedMatch[1] // Berisi cipher JSON
	keyData := ""
	if len(keyMatch) >= 2 {
		keyData = keyMatch[1]
	}

	// Bersihkan escape character bawaan string jika ada
	rawEmbedURL = strings.ReplaceAll(rawEmbedURL, `\"`, `"`)

	fmt.Printf("[TLS-SCRAPER] Ekstraksi Sukses!\n")

	metadata := entities.NewVideoMetadata(
		postID,
		"Extracted Title",
		rawEmbedURL,
		keyData,
		"",
	)

	return result.Ok(metadata)
}
