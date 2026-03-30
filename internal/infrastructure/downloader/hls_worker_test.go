package downloader

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestHLSDownloader_ParseSegments(t *testing.T) {
	downloader := NewHLSDownloader(1)
	baseURL := "https://mock-server.com/hls/"

	// Simulasi M3U8 dengan komentar, URL absolut, dan URL relatif
	playlist := `#EXTM3U
#EXT-X-VERSION:3
#EXTINF:10.0,
segment1.ts
#EXTINF:10.0,
https://cdn.external.com/segment2.ts
#EXTINF:10.0,
/absolute/path/segment3.ts
`

	expected := []string{
		"https://mock-server.com/hls/segment1.ts",
		"https://cdn.external.com/segment2.ts",
		"https://mock-server.com/hls//absolute/path/segment3.ts",
	}

	segments := downloader.parseSegments(playlist, baseURL)

	if len(segments) != 3 {
		t.Fatalf("TC-7.5.1 GAGAL: Diharapkan 3 segmen, mendapatkan %d segmen", len(segments))
	}

	if !reflect.DeepEqual(segments, expected) {
		t.Errorf("TC-7.5.2 GAGAL: Hasil ekstraksi URL tidak presisi.\nDiharapkan: %v\nMendapatkan: %v", expected, segments)
	}
	t.Log("✅ TC-7.5.1 & TC-7.5.2 LULUS: Parsing Playlist M3U8 sukses")
}

func TestHLSDownloader_MasterPlaylistResolution(t *testing.T) {
	mux := http.NewServeMux()

	// Endpoint Master Playlist
	mux.HandleFunc("/master.m3u8", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`#EXTM3U
#EXT-X-STREAM-INF:BANDWIDTH=800000,RESOLUTION=640x360
/media_360p.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=1400000,RESOLUTION=842x480
/media_480p.m3u8
`))
	})

	mux.HandleFunc("/media_360p.m3u8", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`#EXTM3U
#EXTINF:10.0,
/seg1.ts
`))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	downloader := NewHLSDownloader(1)

	playlistContent, _, err := downloader.fetchPlaylist(server.URL + "/master.m3u8")
	if err != nil {
		t.Fatalf("Gagal memanggil fetchPlaylist: %v", err)
	}

	if !strings.Contains(playlistContent, "/seg1.ts") {
		t.Errorf("GAGAL: Resolusi rekursif Master Playlist tidak berhasil mencapai Media Playlist. Mendapatkan: \n%s", playlistContent)
	} else {
		t.Log("✅ FITUR BARU LULUS: Resolusi rekursif Master -> Media Playlist berhasil!")
	}
}

func TestHLSDownloader_SequentialAssemblyAndConcurrency(t *testing.T) {
	mux := http.NewServeMux()
	numSegments := 20
	expectedContent := ""

	for i := 0; i < numSegments; i++ {
		i := i
		content := fmt.Sprintf("SEG[%d]", i)
		expectedContent += content

		mux.HandleFunc(fmt.Sprintf("/seg%d.ts", i), func(w http.ResponseWriter, r *http.Request) {
			delay := time.Duration((numSegments-i)%5) * 10 * time.Millisecond
			time.Sleep(delay)
			w.Write([]byte(content))
		})
	}

	mux.HandleFunc("/playlist.m3u8", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("#EXTM3U\n"))
		for i := 0; i < numSegments; i++ {
			w.Write([]byte(fmt.Sprintf("#EXTINF:10.0,\n/seg%d.ts\n", i)))
		}
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	downloader := NewHLSDownloader(10)
	outputFile := filepath.Join(t.TempDir(), "output_stress.ts")

	err := downloader.DownloadVideo(server.URL+"/playlist.m3u8", outputFile)
	if err != nil {
		t.Fatalf("Tidak diharapkan error selama proses normal: %v", err)
	}

	data, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("Gagal membaca file output: %v", err)
	}

	if string(data) != expectedContent {
		t.Errorf("TC-7.5.3 GAGAL: Urutan biner berantakan. Implementasi buffer perakitan gagal menangani Out-Of-Order.")
	} else {
		t.Logf("✅ TC-7.5.3 LULUS: Perakitan sekuensial berhasil sempurna di bawah eksekusi konkuren!")
	}
}

func TestHLSDownloader_NetworkResiliency(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("/seg0.ts", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("DATA_0")) })

	mux.HandleFunc("/seg1.ts", func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if ok {
			conn, _, _ := hj.Hijack()
			conn.Close()
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	})

	mux.HandleFunc("/playlist.m3u8", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("#EXTM3U\n#EXTINF:10.0,\n/seg0.ts\n#EXTINF:10.0,\n/seg1.ts\n"))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	downloader := NewHLSDownloader(3)
	outputFile := filepath.Join(t.TempDir(), "output_error.ts")

	err := downloader.DownloadVideo(server.URL+"/playlist.m3u8", outputFile)

	if err == nil {
		t.Errorf("❌ TC-7.5.4 GAGAL: Error jaringan tidak dikembalikan ke fungsi pemanggil. Proses gagal dibatalkan dengan aman.")
	} else {
		t.Logf("✅ TC-7.5.4 LULUS: Sistem berhasil menangkap error jaringan dengan aman: %v", err)
	}
}
