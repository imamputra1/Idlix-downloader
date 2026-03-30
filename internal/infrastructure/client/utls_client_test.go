//go:build integration

package client_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/imamputra1/idlix-downloader/internal/infrastructure/client"
)

func TestAntiBotClient_StatefulMemory(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/set-cookie":
			http.SetCookie(w, &http.Cookie{
				Name:  "auth_token",
				Value: "rahasia123",
				Path:  "/",
			})
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Cookie set"))

		case "/check-cookie":
			cookie, err := r.Cookie("auth_token")
			if err != nil || cookie.Value != "rahasia123" {
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte("Missing or invalid cookie"))
				return
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Cookie is valid"))

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockServer.Close()

	utlsClient := client.NewAntiBotClient()

	t.Log("Menjalankan Request 1: GET /set-cookie")
	resp1, err := utlsClient.Get(mockServer.URL + "/set-cookie")
	if err != nil {
		t.Fatalf("Request 1 gagal: %v", err)
	}
	defer resp1.Body.Close()

	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("DoD Gagal: Request 1 mengembalikan status %d, diharapkan 200", resp1.StatusCode)
	}

	t.Log("Menjalankan Request 2: GET /check-cookie")
	resp2, err := utlsClient.Get(mockServer.URL + "/check-cookie")
	if err != nil {
		t.Fatalf("Request 2 gagal: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode == http.StatusUnauthorized {
		t.Fatal("DoD Gagal: Stateful Memory tidak bekerja. Klien tidak mengirimkan cookie (HTTP 401). Pastikan CookieJar sudah terpasang di http.Client.")
	}

	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("DoD Gagal: Request 2 mengembalikan status %d, diharapkan 200", resp2.StatusCode)
	}

	t.Log("Skenario 1 PASS: Stateful Memory berfungsi dengan sempurna!")
}

func TestAntiBotClient_TLSRegression(t *testing.T) {
	targetURL := "https://tv12.idlixku.com/"

	utlsClient := client.NewAntiBotClient()

	t.Logf("Menjalankan Request Regresi ke: %s", targetURL)
	req, err := http.NewRequest(http.MethodGet, targetURL, nil)
	if err != nil {
		t.Fatalf("Gagal membuat request regresi: %v", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")

	resp, err := utlsClient.Do(req)
	if err != nil {
		t.Fatalf("Penetrasi jaringan gagal (Network/TLS Error): %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusServiceUnavailable {
		t.Fatalf("DoD Gagal: Regresi terdeteksi! Klien diblokir oleh WAF (HTTP %d). Konfigurasi CookieJar mungkin mengganggu Transport uTLS.", resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("DoD Gagal: Mendapat status %d, diharapkan 200 OK", resp.StatusCode)
	}

	t.Log("Skenario 2 PASS: Penyamaran uTLS tetap ampuh walau dipasangi CookieJar!")
}
