package scraper

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/imamputra1/idlix-downloader/internal/core/entities"
	"github.com/imamputra1/idlix-downloader/internal/core/ports"
	"github.com/imamputra1/idlix-downloader/pkg/result"
)

type GoQueryScraper struct {
	client *http.Client
}

func NewGoQueryScraper(client *http.Client) ports.Scraper {
	return &GoQueryScraper{
		client: client,
	}
}

func (s *GoQueryScraper) ScraperMetadata(targetURL string) result.Result[entities.VideoMetadata] {
	req, err := http.NewRequest(http.MethodGet, targetURL, nil)
	if err != nil {
		return result.Err[entities.VideoMetadata](fmt.Errorf("failed to create GET request: %w", err))
	}

	// Menyamakan Header dengan referensi sesi navigasi asli
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/127.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9,id;q=0.8")

	resp, err := s.client.Do(req)
	if err != nil {
		return result.Err[entities.VideoMetadata](fmt.Errorf("network error during GET: %w", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return result.Err[entities.VideoMetadata](fmt.Errorf("unexpected status code: %d", resp.StatusCode))
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return result.Err[entities.VideoMetadata](fmt.Errorf("failed to parse HTML DOM: %w", err))
	}

	postID := ""
	nonce := ""

	// =========================================================================
	// 1. EKSTRAKSI POST ID
	// =========================================================================
	if val, exists := doc.Find("meta#dooplay-ajax-counter").Attr("data-postid"); exists {
		postID = val
	}

	// Fallback ID
	if postID == "" {
		doc.Find("input#postid, input[name='postid'], input#post_id, input[name='post_id'], meta[name='post_id']").Each(func(i int, sel *goquery.Selection) {
			if val, exists := sel.Attr("value"); exists && postID == "" {
				postID = val
			} else if val, exists := sel.Attr("content"); exists && postID == "" {
				postID = val
			}
		})
	}

	if postID == "" {
		return result.Err[entities.VideoMetadata](errors.New("failed to find post ID in the DOM"))
	}

	// =========================================================================
	// 2. EKSTRAKSI NONCE (WAF/Decoy Bypass)
	// =========================================================================
	// Mencari nonce dari dalam tag script (biasanya disembunyikan di variabel JS dtAjax)
	doc.Find("script").Each(func(i int, sel *goquery.Selection) {
		if nonce != "" {
			return // Jika sudah ketemu, skip iterasi
		}
		text := sel.Text()

		// Deteksi pola penulisan: "nonce":"1234abcd" atau 'nonce':'1234abcd'
		re := regexp.MustCompile(`(?i)["']nonce["']\s*:\s*["']([^"']+)["']`)
		matches := re.FindStringSubmatch(text)
		if len(matches) > 1 {
			nonce = matches[1]
		}
	})

	// Fallback Nonce: Kadang disisipkan sebagai atribut data pada elemen HTML
	if nonce == "" {
		if val, exists := doc.Find("#dooplay-ajax-counter, .dooplay-ajax-counter").Attr("data-nonce"); exists {
			nonce = val
		}
	}

	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		return result.Err[entities.VideoMetadata](fmt.Errorf("invalid target URL: %w", err))
	}
	ajaxURL := fmt.Sprintf("%s://%s/wp-admin/admin-ajax.php", parsedURL.Scheme, parsedURL.Host)

	// =========================================================================
	// 3. PERAKITAN PAYLOAD AJAX
	// =========================================================================
	payload := url.Values{}
	payload.Set("action", "doo_player_ajax")
	payload.Set("post", postID)
	payload.Set("nume", "1")
	payload.Set("type", "movie")

	// Menyuntikkan token keamanan untuk membuktikan kita bukan bot
	if nonce != "" {
		payload.Set("nonce", nonce)
	}

	fmt.Printf("\n================ DEBUGGING AJAX =================\n")
	fmt.Printf("1. Target URL : %s\n", targetURL)
	fmt.Printf("2. Post ID    : '%s'\n", postID)
	fmt.Printf("3. Nonce      : '%s' (Jika kosong, siap-siap dapat honeypot!)\n", nonce)
	fmt.Printf("4. Payload    : %s\n", payload.Encode())
	fmt.Printf("=================================================\n\n")

	ajaxReq, err := http.NewRequest(http.MethodPost, ajaxURL, strings.NewReader(payload.Encode()))
	if err != nil {
		return result.Err[entities.VideoMetadata](fmt.Errorf("failed to create AJAX POST request: %w", err))
	}

	origin := fmt.Sprintf("%s://%s", parsedURL.Scheme, parsedURL.Host)
	ajaxReq.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	ajaxReq.Header.Set("X-Requested-With", "XMLHttpRequest")
	ajaxReq.Header.Set("User-Agent", req.Header.Get("User-Agent"))
	ajaxReq.Header.Set("Referer", targetURL)
	ajaxReq.Header.Set("Origin", origin)
	ajaxReq.Header.Set("Accept", "*/*")
	ajaxReq.Header.Set("Accept-Language", req.Header.Get("Accept-Language"))

	ajaxResp, err := s.client.Do(ajaxReq)
	if err != nil {
		return result.Err[entities.VideoMetadata](fmt.Errorf("network error during AJAX POST: %w", err))
	}
	defer ajaxResp.Body.Close()

	if ajaxResp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(ajaxResp.Body)
		return result.Err[entities.VideoMetadata](fmt.Errorf("AJAX request failed with status %d. Server says: %s", ajaxResp.StatusCode, string(errBody)))
	}

	ajaxBody, err := io.ReadAll(ajaxResp.Body)
	if err != nil {
		return result.Err[entities.VideoMetadata](fmt.Errorf("failed to read AJAX response: %w", err))
	}
	fmt.Printf("\n[DEBUG] Raw AJAX response dari aggregator: %s\n\n", string(ajaxBody))

	var ajaxResult struct {
		EmbedURL string `json:"embed_url"`
		Key      string `json:"key"`
	}

	if err := json.Unmarshal(ajaxBody, &ajaxResult); err != nil {
		return result.Err[entities.VideoMetadata](fmt.Errorf("failed to parse JSON from AJAX response: %w. Raw Data: %s", err, string(ajaxBody)))
	}

	if ajaxResult.EmbedURL == "" {
		return result.Err[entities.VideoMetadata](errors.New("kunci 'embed_url' kosong atau tidak ditemukan dalam respons JSON agregator"))
	}

	metadata := entities.NewVideoMetadata(
		postID,
		"Unknown Title",
		ajaxResult.EmbedURL,
		ajaxResult.Key,
		"",
	)

	return result.Ok(metadata)
}
