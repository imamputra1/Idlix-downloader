package ports

import (
	"github.com/imamputra1/idlix-downloader/pkg/result"
)

type CryptoEngine interface {
	DecryptURL(ciphertext string, secretKey string) result.Result[string]
}
