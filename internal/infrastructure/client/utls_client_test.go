//go:build integration

// ============================================================================
// File: internal/infrastructure/client/utls_client_test.go
// ============================================================================
package client_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/imamputra1/idlix-downloader/internal/infrastructure/client"
)

// Skenario 1: Persistensi Sesi Dua Arah (Stateful Memory Test)
func TestAntiBotClient_StatefulMemory(t *testing.T) {
	// 1. Setup Mock Server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/set-cookie":
			// Peladen menanamkan cookie
			http.SetCookie(w, &http.Cookie{
				Name:  "auth_token",
				Value: "rahasia123",
				Path:  "/",
			})
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Cookie set"))

		case "/check-cookie":
			// Peladen memvalidasi keberadaan cookie
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

	// 2. Inisialisasi Klien uTLS (dengan CookieJar yang sudah Anda tambahkan)
	utlsClient := client.NewAntiBotClient()

	// 3. Request Pertama: Menembak /set-cookie
	t.Log("Menjalankan Request 1: GET /set-cookie")
	resp1, err := utlsClient.Get(mockServer.URL + "/set-cookie")
	if err != nil {
		t.Fatalf("Request 1 gagal: %v", err)
	}
	defer resp1.Body.Close()

	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("DoD Gagal: Request 1 mengembalikan status %d, diharapkan 200", resp1.StatusCode)
	}

	// 4. Request Kedua: Menembak /check-cookie (Menggunakan instance klien yang SAMA)
	t.Log("Menjalankan Request 2: GET /check-cookie")
	resp2, err := utlsClient.Get(mockServer.URL + "/check-cookie")
	if err != nil {
		t.Fatalf("Request 2 gagal: %v", err)
	}
	defer resp2.Body.Close()

	// 5. Asersi DoD
	if resp2.StatusCode == http.StatusUnauthorized {
		t.Fatal("DoD Gagal: Stateful Memory tidak bekerja. Klien tidak mengirimkan cookie (HTTP 401). Pastikan CookieJar sudah terpasang di http.Client.")
	}

	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("DoD Gagal: Request 2 mengembalikan status %d, diharapkan 200", resp2.StatusCode)
	}

	t.Log("Skenario 1 PASS: Stateful Memory berfungsi dengan sempurna!")
}

// Skenario 2: Uji Regresi Penyamaran TLS (Regression Test)
func TestAntiBotClient_TLSRegression(t *testing.T) {
	// Ganti dengan homepage agregator yang valid untuk memastikan WAF Bypass masih bekerja
	targetURL := "https://tv12.idlixku.com/"

	utlsClient := client.NewAntiBotClient()

	t.Logf("Menjalankan Request Regresi ke: %s", targetURL)
	req, err := http.NewRequest(http.MethodGet, targetURL, nil)
	if err != nil {
		t.Fatalf("Gagal membuat request regresi: %v", err)
	}

	// Gunakan header standar browser agar tidak diblokir karena missing User-Agent
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")

	resp, err := utlsClient.Do(req)
	if err != nil {
		t.Fatalf("Penetrasi jaringan gagal (Network/TLS Error): %v", err)
	}
	defer resp.Body.Close()

	// Asersi DoD
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusServiceUnavailable {
		t.Fatalf("DoD Gagal: Regresi terdeteksi! Klien diblokir oleh WAF (HTTP %d). Konfigurasi CookieJar mungkin mengganggu Transport uTLS.", resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("DoD Gagal: Mendapat status %d, diharapkan 200 OK", resp.StatusCode)
	}

	t.Log("Skenario 2 PASS: Penyamaran uTLS tetap ampuh walau dipasangi CookieJar!")
}
