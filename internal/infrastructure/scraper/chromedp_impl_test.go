//go:build integration

package scraper_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/imamputra1/idlix-downloader/internal/infrastructure/scraper"
)

// setupMockServer membangun server lokal tiruan yang menyimulasikan berbagai perilaku target.
func setupMockServer(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()

	// Endpoint untuk Kriteria 1 & 2 (Happy Path)
	mux.HandleFunc("/happy-path", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		// Mengembalikan DOM sesuai spesifikasi Kriteria 1
		w.Write([]byte(`
			<html>
			<head>
				<meta id="dooplay-ajax-counter" data-postid="9999">
			</head>
			<body><h1>Mock Video Page</h1></body>
			</html>
		`))
	})

	// Endpoint AJAX untuk Kriteria 1 & 2 (Routing & Header Validation)
	mux.HandleFunc("/wp-admin/admin-ajax.php", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Kriteria 2 GAGAL: Diharapkan POST request, mendapatkan %s", r.Method)
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Validasi Kriteria 2: Pengecekan Header Ketat
		contentType := r.Header.Get("Content-Type")
		if contentType != "application/x-www-form-urlencoded; charset=UTF-8" {
			t.Errorf("Kriteria 2 GAGAL: Invalid Content-Type Header: %s", contentType)
		}

		xReqWith := r.Header.Get("X-Requested-With")
		if xReqWith != "XMLHttpRequest" {
			t.Errorf("Kriteria 2 GAGAL: Invalid X-Requested-With Header: %s", xReqWith)
		}

		// Validasi Kriteria 1: Memastikan In-Browser Fetch membawa PostID yang benar
		r.ParseForm()
		postID := r.FormValue("post")
		if postID != "9999" {
			t.Errorf("Kriteria 1 GAGAL: Diharapkan post ID '9999', mendapatkan '%s'", postID)
		}

		// Mereturn respons JSON yang akan diparsing oleh ChromedpScraper
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"embed_url": "https://mock-embed.url/master.m3u8", "key": "mock-key-123"}`))
	})

	// Endpoint untuk Kriteria 3 (Timeout Handling)
	mux.HandleFunc("/timeout-trap", func(w http.ResponseWriter, r *http.Request) {
		// Menahan respons selama 35 detik (Melebihi batas context Chrome 30 detik)
		time.Sleep(35 * time.Second)
		w.Write([]byte(`<html><body>Terlambat!</body></html>`))
	})

	// Endpoint untuk Kriteria 4 (Empty DOM / Error Catching)
	mux.HandleFunc("/empty-dom", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><body><h1>Tidak ada Post ID disini</h1></body></html>`))
	})

	return httptest.NewServer(mux)
}

func TestChromedpScraper_Criteria(t *testing.T) {
	// 1. Inisiasi Mock Server & Scraper
	ts := setupMockServer(t)
	defer ts.Close()

	scraperSvc := scraper.NewChromedpScraper()

	// =========================================================================
	// PENGUJIAN KRITERIA 1 & 2 (Happy Path & Header Routing)
	// =========================================================================
	t.Run("Kriteria 1 & 2: Presisi Ekstraksi & In-Browser Fetch Validation", func(t *testing.T) {
		targetURL := ts.URL + "/happy-path"
		res := scraperSvc.ScraperMetadata(targetURL)

		if res.IsErr() {
			_, err := res.Unwrap()
			t.Fatalf("Expected success, tapi mendapatkan error: %v", err)
		}

		metadata, _ := res.Unwrap()

		// Verifikasi ekstraksi PostID
		if metadata.ID() != "9999" {
			t.Errorf("Expected PostID '9999', got '%s'", metadata.ID())
		}

		// Verifikasi ekstraksi EmbedURL
		if metadata.EncryptedEmbedURL() != "https://mock-embed.url/master.m3u8" {
			t.Errorf("Expected EmbedURL 'https://mock-embed.url/master.m3u8', got '%s'", metadata.EncryptedEmbedURL())
		}

		t.Log("Kriteria 1 & 2: LULUS (Happy Path dan Header Validasi Sukses)")
	})

	// =========================================================================
	// PENGUJIAN KRITERIA 4 (Perangkap Logika JS / DOM Kosong)
	// =========================================================================
	t.Run("Kriteria 4: Perangkap Logika JavaScript (Error Catching)", func(t *testing.T) {
		targetURL := ts.URL + "/empty-dom"
		res := scraperSvc.ScraperMetadata(targetURL)

		if !res.IsErr() {
			t.Fatalf("Expected error karena DOM kosong, tapi mendapatkan sukses")
		}

		_, err := res.Unwrap()
		expectedErrMsg := "javascript in-browser error: Post ID tidak ditemukan di DOM"
		if !strings.Contains(err.Error(), expectedErrMsg) {
			t.Errorf("Diharapkan pesan error mengandung '%s', tapi mendapatkan: '%v'", expectedErrMsg, err)
		}

		t.Log("Kriteria 4: LULUS (JS Error berhasil ditangkap oleh Go)")
	})

	// =========================================================================
	// PENGUJIAN KRITERIA 3 (Resiliensi Batas Waktu)
	// =========================================================================
	t.Run("Kriteria 3: Resiliensi Batas Waktu (Timeout Handling)", func(t *testing.T) {
		if testing.Short() {
			t.Skip("Melewati pengujian timeout di mode -short (karena memakan waktu 30 detik)")
		}

		targetURL := ts.URL + "/timeout-trap"

		start := time.Now()
		res := scraperSvc.ScraperMetadata(targetURL)
		duration := time.Since(start)

		if !res.IsErr() {
			t.Fatalf("Expected timeout error, tapi mendapatkan sukses")
		}

		_, err := res.Unwrap()
		expectedErrMsg := "context deadline exceeded"
		if !strings.Contains(err.Error(), expectedErrMsg) {
			t.Errorf("Diharapkan error mengandung '%s', tapi mendapatkan: '%v'", expectedErrMsg, err)
		}

		// Memastikan proses terputus paksa pada kisaran 30 detik (Toleransi 2-3 detik overhead Chrome)
		if duration < 29*time.Second || duration > 34*time.Second {
			t.Errorf("Diharapkan terputus di sekitar 30 detik, tapi memakan waktu: %v", duration)
		}

		t.Logf("Kriteria 3: LULUS (Proses diputus paksa tepat pada %v)", duration)
	})
}
