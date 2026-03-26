//go:build integration

// ============================================================================
// File: internal/infrastructure/client/utls_client_test.go
// ============================================================================
package client_test

import (
	"net/http"
	"testing"

	"github.com/imamputra1/idlix-downloader/internal/infrastructure/client"
)

func TestAntiBotClient_E2EPenetration(t *testing.T) {
	targetURL := "https://www.cloudflare.com"

	httpClient := client.NewAntiBotClient()

	req, err := http.NewRequest(http.MethodGet, targetURL, nil)
	if err != nil {
		t.Fatalf("Gagal merakit HTTP Request: %v", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("Penetrasi Gagal (Jaringan/Handshake Error): %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Penetrasi Gagal (Terblokir WAF): Ekspektasi 200 OK, mendapat %d %s", resp.StatusCode, resp.Status)
	}
}
