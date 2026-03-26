package ports

import (
	"github.com/imamputra1/idlix-downloader/pkg/result"
)

type CryptoEngine interface {
	DescryptURL(ciphertext string, secretKey string) result.Result[string]
}
