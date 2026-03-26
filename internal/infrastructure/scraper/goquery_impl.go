package scraper

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")

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
	doc.Find("input#postid, input[name='postid'], meta[name='post_id']").Each(func(i int, sel *goquery.Selection) {
		if val, exists := sel.Attr("value"); exists && postID == "" {
			postID = val
		} else if val, exists := sel.Attr("content"); exists && postID == "" {
			postID = val
		}
	})

	if postID == "" {
		return result.Err[entities.VideoMetadata](errors.New("failed to find post ID in the DOM"))
	}

	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		return result.Err[entities.VideoMetadata](fmt.Errorf("invalid target URL: %w", err))
	}

	ajaxURL := fmt.Sprintf("%s://%s/wp-admin/admin-ajax.php", parsedURL.Scheme, parsedURL.Host)
	payload := url.Values{}
	payload.Set("action", "get_video_source")
	payload.Set("post_id", postID)

	ajaxReq, err := http.NewRequest(http.MethodPost, ajaxURL, strings.NewReader(payload.Encode()))
	if err != nil {
		return result.Err[entities.VideoMetadata](fmt.Errorf("failed to create AJAX POST request: %w", err))
	}

	ajaxReq.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	ajaxReq.Header.Set("X-Requested-With", "XMLHttpRequest")
	ajaxReq.Header.Set("User-Agent", req.Header.Get("User-Agent"))
	ajaxReq.Header.Set("Referer", targetURL)

	ajaxResp, err := s.client.Do(ajaxReq)
	if err != nil {
		return result.Err[entities.VideoMetadata](fmt.Errorf("network error during AJAX POST: %w", err))
	}
	defer ajaxResp.Body.Close()

	if ajaxResp.StatusCode != http.StatusOK {
		return result.Err[entities.VideoMetadata](fmt.Errorf("AJAX request failed with status: %d", ajaxResp.StatusCode))
	}

	ajaxBody, err := io.ReadAll(ajaxResp.Body)
	if err != nil {
		return result.Err[entities.VideoMetadata](fmt.Errorf("failed to read AJAX response: %w", err))
	}

	var ajaxResult struct {
		Data string `json:"data"`
	}

	if err := json.Unmarshal(ajaxBody, &ajaxResult); err != nil {
		ajaxResult.Data = strings.TrimSpace(string(ajaxBody))
	}

	if ajaxResult.Data == "" {
		return result.Err[entities.VideoMetadata](errors.New("ciphertext payload is empty in AJAX response"))
	}

	metadata := entities.NewVideoMetadata(
		postID,
		"Unknown Title",
		ajaxResult.Data,
		targetURL,
	)

	return result.Ok(metadata)
}
