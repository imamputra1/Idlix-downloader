package downloader

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type VideoResolution struct {
	Bandwidth       int     `json:"bandwidth"`
	Resolution      string  `json:"resolution"`
	PlaylistURL     string  `json:"playlist_url"`
	EstimatedSizeMB float64 `json:"estimated_size_mb"`
}

type HLSDownloader struct {
	client     *http.Client
	workerPool int
}

func NewHLSDownloader(workers int) *HLSDownloader {
	return &HLSDownloader{
		client:     &http.Client{Timeout: 180 * time.Second},
		workerPool: workers,
	}
}

func (h *HLSDownloader) estimated_size_mb(MediaPlaylistURL string) float64 {
	req, _ := http.NewRequest("GET", MediaPlaylistURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64)")
	req.Header.Set("Referer", "https://jeniusplay.com/")

	resp, err := h.client.Do(req)
	if err != nil || resp.StatusCode != 200 {
		return 0
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	extinfRegex := regexp.MustCompile(`#EXTINF:([\d\.]+)`)
	var totalDuration float64

	for scanner.Scan() {
		if match := extinfRegex.FindStringSubmatch(scanner.Text()); len(match) > 1 {
			val, _ := strconv.ParseFloat(match[1], 64)
			totalDuration += val
		}
	}
	return totalDuration
}

func (h *HLSDownloader) ExtractResolutions(masterM3U8 string) ([]VideoResolution, error) {
	req, _ := http.NewRequest("GET", masterM3U8, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64)")
	req.Header.Set("Referer", "https://jeniusplay.com/")

	resp, err := h.client.Do(req)
	if err != nil || resp.StatusCode != 200 {
		return nil, fmt.Errorf("akses master playlist ditolak oleh server")
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	scanner := bufio.NewScanner(strings.NewReader(string(bodyBytes)))

	var resolutions []VideoResolution
	baseURL, _ := url.Parse(masterM3U8)

	bandwidthRegex := regexp.MustCompile(`BANDWIDTH=(\d+)`)
	resolutionRegex := regexp.MustCompile(`RESOLUTION=(\d+x\d+)`)

	var currentBandwidth int
	var currentResString string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "#EXT-X-STREAM-INF") {
			if bwMatch := bandwidthRegex.FindStringSubmatch(line); len(bwMatch) > 1 {
				currentBandwidth, _ = strconv.Atoi(bwMatch[1])
			}
			if resMatch := resolutionRegex.FindStringSubmatch(line); len(resMatch) > 1 {
				currentResString = resMatch[1]
			}
			continue
		}

		if !strings.HasPrefix(line, "#") && currentBandwidth > 0 {
			refURL, _ := url.Parse(line)
			resolvedURL := baseURL.ResolveReference(refURL).String()

			resolutions = append(resolutions, VideoResolution{
				Bandwidth:   currentBandwidth,
				Resolution:  currentResString,
				PlaylistURL: resolvedURL,
			})

			currentBandwidth = 0
			currentResString = ""
		}
	}

	if len(resolutions) == 0 {
		return nil, fmt.Errorf("tidak ada resolusi yang ditemukan")
	}

	totalDurationSec := h.estimated_size_mb(resolutions[0].PlaylistURL)
	if totalDurationSec > 0 {
		for i := range resolutions {
			resolutions[i].EstimatedSizeMB = (float64(resolutions[i].Bandwidth) * totalDurationSec) / 8.0 / 1024.0 / 1024.0
		}
	}
	return resolutions, nil
}

func (h *HLSDownloader) ExecuteDownload(MediaPlaylistURL string, outputFilename string) error {
	fmt.Print("[DOWNLOAD] Menganalisis M3U8 Playlist...\n")

	req, _ := http.NewRequest("GET", MediaPlaylistURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64)")
	req.Header.Set("Referer", "https://jeniusplay.com/")

	resp, err := h.client.Do(req)
	if err != nil || resp.StatusCode != 200 {
		return fmt.Errorf("gagal mengakses sub-playlist target")
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	segments := h.parseSegments(string(bodyBytes), MediaPlaylistURL)

	if len(segments) == 0 {
		return fmt.Errorf("tidak ada segmen video yang ditemukan di dalam sub-playlist")
	}

	tempDir := filepath.Join(filepath.Dir(outputFilename), ".tmp_hls_segments")
	os.MkdirAll(tempDir, 0o755)
	defer os.RemoveAll(tempDir)

	fmt.Print("[DOWNLOAD] Ditemukan %d segmen video. Memulai pasukan pekerja...\n", len(segments))

	var wg sync.WaitGroup
	segmentChan := make(chan struct {
		index int
		url   string
	}, len(segments))
	resultChan := make(chan struct {
		index int
		err   error
	}, len(segments))

	for i := 0; i < h.workerPool; i++ {
		wg.Add(1)
		go h.worker(segmentChan, resultChan, tempDir, &wg)
	}

	for i, segURL := range segments {
		segmentChan <- struct {
			index int
			url   string
		}{index: i, url: segURL}
	}
	close(segmentChan)

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	downloadedCount := 0

	for res := range resultChan {
		if res.err != nil {
			fmt.Printf("\n Kegagalan permanen pada segmen %d: %v\n", res.index, res.err)
			return fmt.Errorf("pengunduhan dibatalkan karena error jaringan yang tidak dapat dipulihkan")
		}
		downloadedCount++
		fmt.Printf("\r[DOWNLOAD] Progress: %d/%d segmen terunduh ke Disk...", downloadedCount, len(segments))
	}

	fmt.Print("\n[DOWNLOAD] Menggabungkan segmen menjadi file Video final...\n")
	outputFile, err := os.Create(outputFilename)
	if err != nil {
		return fmt.Errorf("gagal membuat file final: %w", err)
	}
	defer outputFile.Close()

	for i := 0; i < len(segments); i++ {
		tempFilePath := filepath.Join(tempDir, fmt.Sprintf("seg_%05d.ts", i))
		tempData, err := os.ReadFile(tempFilePath)
		if err != nil {
			return fmt.Errorf("gagal membaca segment %d: %w", i, err)
		}
		outputFile.Write(tempData)
	}
	return nil
}

func (h *HLSDownloader) parseSegments(playlist string, sourceURL string) []string {
	var segments []string
	scanner := bufio.NewScanner(strings.NewReader(playlist))
	baseURL, _ := url.Parse(sourceURL)
	expectSegment := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#EXTINF") {
			expectSegment = true
			continue
		}
		if expectSegment && !strings.HasPrefix(line, "#") {
			refURL, _ := url.Parse(line)
			segments = append(segments, baseURL.ResolveReference(refURL).String())
			expectSegment = false
		}
	}
	return segments
}

func (h *HLSDownloader) worker(jobs <-chan struct {
	index int
	url   string
}, results chan<- struct {
	index int
	err   error
}, tempDir string, wg *sync.WaitGroup,
) {
	defer wg.Done()
	maxRetries := 5

	for job := range jobs {
		var finalErr error
		success := false

		for attempt := 1; attempt <= maxRetries; attempt++ {
			req, _ := http.NewRequest("GET", job.url, nil)
			req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64)")
			req.Header.Set("Referer", "https://jeniusplay.com/")

			resp, err := h.client.Do(req)
			if err != nil {
				finalErr = err
				time.Sleep(time.Duration(attempt*2) * time.Second)
				continue
			}

			if resp.StatusCode != 200 {
				resp.Body.Close()
				finalErr = fmt.Errorf("HTTP Status %d", resp.StatusCode)
				time.Sleep(time.Duration(attempt*2) * time.Second)
				continue
			}

			data, err := io.ReadAll(resp.Body)
			resp.Body.Close()

			if err == nil {
				tempFilePath := filepath.Join(tempDir, fmt.Sprintf("seg_%05d.ts", job.index))
				err = os.WriteFile(tempFilePath, data, 0o644)
				if err == nil {
					success = true
					break
				}
			}

			finalErr = err
			time.Sleep(time.Duration(attempt) * time.Second)
		}

		if !success {
			results <- struct {
				index int
				err   error
			}{index: job.index, err: fmt.Errorf("gagal setelah %d percobaan. Error terakhir: %v", maxRetries, finalErr)}
		} else {
			results <- struct {
				index int
				err   error
			}{index: job.index, err: nil}
		}
	}
}
