package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"errors"

	"github.com/imamputra1/idlix-downloader/internal/core/ports"
	"github.com/imamputra1/idlix-downloader/pkg/result"
)

type AESDecrypter struct{}

func NewAESDecrypter() ports.CryptoEngine {
	return AESDecrypter{}
}

func (a AESDecrypter) DecryptURL(ciphertextBase64 string, secretKey string) result.Result[string] {
	decodedData, err := base64.StdEncoding.DecodeString(ciphertextBase64)
	if err != nil {
		return result.Err[string](errors.New("failed to decode base64 ciphertext: " + err.Error()))
	}
	if len(decodedData) < aes.BlockSize {
		return result.Err[string](errors.New("ciphertext too short to contain IV"))
	}
	keyBytes := []byte(secretKey)
	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return result.Err[string](errors.New("failed to create AES cipher: " + err.Error()))
	}
	iv := decodedData[:aes.BlockSize]
	encryptedPayload := decodedData[aes.BlockSize:]
	if len(encryptedPayload)%aes.BlockSize != 0 {
		return result.Err[string](errors.New("encrypted payload is not a multiple of the block size"))
	}
	mode := cipher.NewCBCDecrypter(block, iv)
	decryptedData := make([]byte, len(encryptedPayload))
	mode.CryptBlocks(decryptedData, encryptedPayload)
	unpaddedData, err := unpadPKCS7(decryptedData)
	if err != nil {
		return result.Err[string](errors.New("failed to remove PKCS7 padding: " + err.Error()))
	}
	return result.Ok(string(unpaddedData))
}

func unpadPKCS7(data []byte) ([]byte, error) {
	length := len(data)
	if length == 0 {
		return nil, errors.New("data is empty")
	}
	paddingLen := int(data[length-1])
	if paddingLen == 0 || paddingLen > length {
		return nil, errors.New("invalid padding length")
	}

	for i := range paddingLen {
		if data[length-1-i] != byte(paddingLen) {
			return nil, errors.New("invalid padding bytes")
		}
	}
	return data[:length-paddingLen], nil
}
