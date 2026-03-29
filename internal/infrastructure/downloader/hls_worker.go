package downloader

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type HLSDownloader struct {
	client     *http.Client
	workerPool int
}

func NewHSLDownloader(workers int) *HLSDownloader {
	return &HLSDownloader{
		client:     &http.Client{},
		workerPool: workers,
	}
}

func (h *HLSDownloader) DownloadVideo(m3u8URL string, outputFilename string) error {
	fmt.Printf("[DOWNLOAD] Membaca M3U8 Playlist... ")

	playlist, baseURL, err := h.fetchPlaylist(m3u8URL)
	if err != nil {
		return err
	}

	segments := h.parseSegments(playlist, baseURL)
	if len(segments) == 0 {
		return fmt.Errorf("tidak ada segmen video yang ditemukan di dalam playlist")
	}
	fmt.Printf("[DOWNLOAD] Ditemukan %d segmen video. Memulai pengunduhan paralel .. \n", len(segments))

	outputFile, err := os.Create(outputFilename)
	if err != nil {
		return fmt.Errorf("gagal membuat file output: %w", err)
	}
	defer outputFile.Close()

	var wg sync.WaitGroup
	segmentChan := make(chan struct {
		index int
		url   string
	}, len(segments))

	resultChan := make(chan struct {
		index int
		data  []byte
		err   error
	}, len(segments))

	for i := 0; i < h.workerPool; i++ {
		wg.Add(1)
		go h.worker(segmentChan, resultChan, &wg)
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

	buffer := make(map[int][]byte)
	expectedIndex := 0
	downloadedCount := 0

	for res := range resultChan {
		if res.err != nil {
			fmt.Printf("\n Pekerja melaporkan kegagalan pada segmen %d: %v\n", res.index, res.err)
			return fmt.Errorf("pengunduhan gagal pada segment %d: %w", res.index, res.err)
		}

		buffer[res.index] = res.data
		downloadedCount++
		fmt.Printf("\r[DOWNLOAD] progress: %d/%d segmen terunduh ...", downloadedCount, len(segments))

		for {
			if data, exists := buffer[expectedIndex]; exists {
				outputFile.Write(data)
				delete(buffer, expectedIndex)
				expectedIndex++
			} else {
				break
			}
		}
	}
	fmt.Printf("\n[DOWNLOAD] PENGUNDUHAN SELESAI! Video tersimpan sebagai: %s\n", outputFilename)

	return nil
}

func (h *HLSDownloader) fetchPlaylist(playlistURL string) (string, string, error) {
	req, err := http.NewRequest("GET", playlistURL, nil)
	if err != nil {
		return "", "", err
	}

	req.Header.Set("User-Agent", "Mozila/5.0 (Windows NT 10.0; Win64, x64) AppleWebKit/537.36")

	resp, err := h.client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)

	parsedURL, _ := url.Parse(playlistURL)
	baseURL := fmt.Sprintf("%s://%s%s/", parsedURL.Scheme, parsedURL.Host, filepath.Dir(parsedURL.Path))

	return string(body), baseURL, err
}

func (h *HLSDownloader) parseSegments(playlist string, baseURL string) []string {
	var segments []string
	scanner := bufio.NewScanner(strings.NewReader(playlist))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if !strings.HasPrefix(line, "http") {
			line = baseURL + line
		}
		segments = append(segments, line)
	}
	return segments
}

func (h *HLSDownloader) worker(jobs <-chan struct {
	index int
	url   string
}, results chan<- struct {
	index int
	data  []byte
	err   error
}, wg *sync.WaitGroup,
) {
	defer wg.Done()

	for job := range jobs {
		req, _ := http.NewRequest("GET", job.url, nil)
		req.Header.Set("User-Agent", "Mozila /5.0")

		resp, err := h.client.Do(req)
		if err != nil {
			results <- struct {
				index int
				data  []byte
				err   error
			}{index: job.index, err: err}
			continue
		}
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()

		results <- struct {
			index int
			data  []byte
			err   error
		}{index: job.index, data: data, err: err}
	}
}
