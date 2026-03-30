package scraper

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/imamputra1/idlix-downloader/internal/core/entities"
	"github.com/imamputra1/idlix-downloader/internal/core/ports"
	"github.com/imamputra1/idlix-downloader/pkg/result"
)

type NetworkSniffer struct {
	Proxied http.RoundTripper
}

func (ns *NetworkSniffer) RoundTrip(req *http.Request) (*http.Response, error) {
	fmt.Printf("\n[SNIFFER] MENGIRIM: %s %s\n", req.Method, req.URL.String())

	start := time.Now()
	resp, err := ns.Proxied.RoundTrip(req)
	duration := time.Since(start)

	if err != nil {
		fmt.Printf("[SNIFFER] GAGAL (%s): %v\n", duration, err)
		return resp, err
	}

	fmt.Printf("[SNIFFER] DITERIMA: %s (Status: %d, Waktu: %s)\n", req.URL.Host, resp.StatusCode, duration)

	if strings.Contains(req.URL.Path, "/wp-admin/admin-ajax.php") {
		bodyBytes, _ := io.ReadAll(resp.Body)
		fmt.Printf("[SNIFFER] PAYLOAD AJAX DITEMUKAN (Panjang: %d bytes):\n%s\n", len(bodyBytes), string(bodyBytes))
		resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	}

	return resp, nil
}

type NativeHTTPScraper struct {
	client *http.Client
}

func NewNativeHTTPScraper() ports.Scraper {
	jar, _ := cookiejar.New(nil)

	baseTransport := &http.Transport{
		ForceAttemptHTTP2: false,
		TLSNextProto:      make(map[string]func(authority string, c *tls.Conn) http.RoundTripper),
	}

	snifferTransport := &NetworkSniffer{
		Proxied: baseTransport,
	}

	return &NativeHTTPScraper{
		client: &http.Client{
			Transport: snifferTransport,
			Jar:       jar,
		},
	}
}

func (s *NativeHTTPScraper) ScraperMetadata(targetURL string) result.Result[entities.VideoMetadata] {
	req, err := http.NewRequest(http.MethodGet, targetURL, nil)
	if err != nil {
		return result.Err[entities.VideoMetadata](fmt.Errorf("gagal membuat request: %w", err))
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "*/*")

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

	postIDRegex := regexp.MustCompile(`p=(\d+)"|data-postid="(\d+)"|postid-(\d+)`)
	postIDMatch := postIDRegex.FindStringSubmatch(htmlContent)
	var postID string
	for _, match := range postIDMatch[1:] {
		if match != "" {
			postID = match
			break
		}
	}

	nonceRegex := regexp.MustCompile(`"nonce":"([^"]+)"|data-nonce="([^"]+)"`)
	nonceMatch := nonceRegex.FindStringSubmatch(htmlContent)
	var nonce string
	for _, match := range nonceMatch[1:] {
		if match != "" {
			nonce = match
			break
		}
	}

	if postID == "" {
		return result.Err[entities.VideoMetadata](fmt.Errorf("Post ID tidak ditemukan"))
	}

	fmt.Printf("[HTTP-SCRAPER] Fase 1 Sukses! PostID: %s | Nonce: %s\n", postID, nonce)

	parsedURL, _ := url.Parse(targetURL)
	ajaxURL := fmt.Sprintf("%s://%s/wp-admin/admin-ajax.php", parsedURL.Scheme, parsedURL.Host)

	var rawPayload string
	if nonce != "" {
		rawPayload = fmt.Sprintf("action=doo_player_ajax&post=%s&nume=1&type=movie&nonce=%s", postID, nonce)
	} else {
		rawPayload = fmt.Sprintf("action=doo_player_ajax&post=%s&nume=1&type=movie", postID)
	}

	postReq, err := http.NewRequest(http.MethodPost, ajaxURL, strings.NewReader(rawPayload))
	if err != nil {
		return result.Err[entities.VideoMetadata](fmt.Errorf("gagal membuat POST: %w", err))
	}

	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	postReq.Header.Set("X-Requested-With", "XMLHttpRequest")
	postReq.Header.Set("Referer", targetURL)
	postReq.Header.Set("User-Agent", req.Header.Get("User-Agent"))
	postReq.Header.Set("Accept", "*/*")

	postResp, err := s.client.Do(postReq)
	if err != nil {
		return result.Err[entities.VideoMetadata](fmt.Errorf("network error Fase 2: %w", err))
	}
	defer postResp.Body.Close()

	ajaxBytes, _ := io.ReadAll(postResp.Body)
	rawResponse := string(ajaxBytes)

	if postResp.StatusCode != 200 || len(strings.TrimSpace(rawResponse)) == 0 {
		return result.Err[entities.VideoMetadata](fmt.Errorf("Silent Drop / HTTP %d", postResp.StatusCode))
	}

	var dooPlayResponse struct {
		EmbedURL string `json:"embed_url"`
		Key      string `json:"key"`
	}

	if err := json.Unmarshal(ajaxBytes, &dooPlayResponse); err != nil {
		return result.Err[entities.VideoMetadata](fmt.Errorf("gagal memparsing JSON. Raw: %s", rawResponse))
	}

	cleanKey := dooPlayResponse.Key
	if strings.Contains(cleanKey, "\\x") {
		if unquoted, err := strconv.Unquote(`"` + cleanKey + `"`); err == nil {
			cleanKey = unquoted
		}
	}

	fmt.Printf("[HTTP-SCRAPER] Ekstraksi Sukses! Panjang CT: %d | Key Bersih: %s\n", len(dooPlayResponse.EmbedURL), cleanKey[:min(10, len(cleanKey))]+"...")

	metadata := entities.NewVideoMetadata(
		postID,
		"Extracted Title",
		dooPlayResponse.EmbedURL,
		cleanKey,
		"",
	)

	return result.Ok(metadata)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
