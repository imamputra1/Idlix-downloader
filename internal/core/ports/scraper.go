package ports

import (
	"github.com/imamputra1/idlix-downloader/internal/core/entities"
	"github.com/imamputra1/idlix-downloader/pkg/result"
)

type Scraper interface {
	ScraperMetadata(url string) result.Result[entities.VideoMetadata]
}
