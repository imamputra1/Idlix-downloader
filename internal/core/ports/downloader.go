package ports

import (
	"github.com/imamputra1/idlix-downloader/internal/core/entities"
	"github.com/imamputra1/idlix-downloader/pkg/result"
)

type HLSClient interface {
	FetchPlaylist(url string) result.Result[entities.HLSPlaylist]
	DownloadSegments(playlist entities.HLSPlaylist, outputDir string) result.Result[string]
}
