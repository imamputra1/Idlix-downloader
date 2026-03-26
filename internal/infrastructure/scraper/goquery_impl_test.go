//go:build integration

package scraper_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/imamputra1/idlix-downloader/internal/infrastructure/scraper"
)

func TestGoQueryScraper_Integration(t *testing.T) {
	expectedPostID := "98765"
	expectedCiphertext := "U2FsdGVkX1+vRandomBase64AESCiphertextData=="

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/movie/target-movie-url":
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			html := `<!DOCTYPE html>
			<html>
			<head><title>Movie Stream</title></head>
			<body>
				<div class="player-container">
					<input type="hidden" id="postid" value="` + expectedPostID + `">
				</div>
			</body>
			</html>`
			w.Write([]byte(html))

		case "/wp-admin/admin-ajax.php":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			r.ParseForm()
			if r.FormValue("post_id") != expectedPostID || r.FormValue("action") != "get_video_source" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			jsonResp := map[string]string{"data": expectedCiphertext}
			json.NewEncoder(w).Encode(jsonResp)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockServer.Close()

	injectedClient := &http.Client{Timeout: 5 * time.Second}
	domScraper := scraper.NewGoQueryScraper(injectedClient)
	targetURL := mockServer.URL + "/movie/target-movie-url"

	res := domScraper.ScraperMetadata(targetURL)

	if res.IsErr() {
		err, _ := res.Unwrap()
		t.Fatalf("Expected successful extraction, got error: %v", err)
	}

	metadata, err := res.Unwrap()
	if err != nil {
		t.Fatalf("Unexpected error unwrapping result: %v", err)
	}

	if metadata.ID() != expectedPostID {
		t.Errorf("Expected Post ID %s, got %s", expectedPostID, metadata.ID())
	}

	if metadata.EncryptedEmbedURL() != expectedCiphertext {
		t.Errorf("Expected Ciphertext %s, got %s", expectedCiphertext, metadata.EncryptedEmbedURL())
	}
}
