package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/imamputra1/idlix-downloader/internal/core/ports"
	"github.com/imamputra1/idlix-downloader/pkg/result"
)

type AESDecrypter struct{}

func NewAESDecrypter() ports.CryptoEngine {
	return AESDecrypter{}
}

type embedData struct {
	Ct string `json:"ct"`
	Iv string `json:"iv"`
	S  string `json:"s"`
	M  string `json:"m"`
}

func (a AESDecrypter) DecryptURL(embedURLJSON string, outerKey string) result.Result[string] {
	var data embedData
	if err := json.Unmarshal([]byte(embedURLJSON), &data); err != nil {
		return result.Err[string](errors.New("failed to unmarshal embed_url JSON: " + err.Error()))
	}

	passphrase, err := generatePassphrase(outerKey, data.M)
	if err != nil {
		return result.Err[string](errors.New("failed to sanitize key/generate passphrase: " + err.Error()))
	}

	salt, err := hex.DecodeString(data.S)
	if err != nil {
		return result.Err[string](errors.New("failed to decode salt hex: " + err.Error()))
	}

	// 1. KDF hanya digunakan untuk menghasilkan Key 32-byte
	aesKey := evpKDF(passphrase, salt)

	// 2. KEMBALI MENGGUNAKAN IV DARI JSON (Skenario 4 akan lulus)
	iv, err := hex.DecodeString(data.Iv)
	if err != nil {
		return result.Err[string](errors.New("failed to decode IV hex: " + err.Error()))
	}

	ctBytes, err := base64.StdEncoding.DecodeString(data.Ct)
	if err != nil {
		return result.Err[string](errors.New("failed to decode ct base64: " + err.Error()))
	}

	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return result.Err[string](errors.New("failed to create AES cipher: " + err.Error()))
	}

	if len(ctBytes)%aes.BlockSize != 0 {
		return result.Err[string](errors.New("ciphertext is not a multiple of block size"))
	}

	// Gunakan IV murni dari JSON untuk dekripsi
	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(ctBytes, ctBytes)

	unpadded, err := unpadPKCS7(ctBytes)
	if err != nil {
		return result.Err[string](errors.New("PKCS7 unpad failed: " + err.Error()))
	}

	var finalURL string
	if err := json.Unmarshal(unpadded, &finalURL); err != nil {
		finalURL = string(unpadded)
	}

	return result.Ok(finalURL)
}

func generatePassphrase(r, e string) ([]byte, error) {
	var rList []string

	for i := 2; i < len(r); i += 4 {
		end := i + 2
		if end > len(r) {
			end = len(r)
		}
		rList = append(rList, r[i:end])
	}

	runes := []rune(e)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	eRev := string(runes)

	pad := (4 - (len(eRev) % 4)) % 4
	eRev += strings.Repeat("=", pad)

	decodedMBytes, err := base64.StdEncoding.DecodeString(eRev)
	if err != nil {
		return nil, errors.New("base64 decode error on m: " + err.Error())
	}

	mList := strings.Split(string(decodedMBytes), "|")
	var passphraseBuilder strings.Builder

	for _, s := range mList {
		if s == "" {
			continue
		}

		isDigit := true
		for _, c := range s {
			if c < '0' || c > '9' {
				isDigit = false
				break
			}
		}
		if !isDigit {
			continue
		}

		idx, err := strconv.Atoi(s)
		if err == nil && idx >= 0 && idx < len(rList) {
			val, err := strconv.ParseInt(rList[idx], 16, 32)
			if err != nil {
				return nil, errors.New("hex parse error: " + err.Error())
			}
			passphraseBuilder.WriteRune(rune(val))
		}
	}

	// BLOK BASE64 DECODE TELAH DIHAPUS. KITA MENGGUNAKAN RAW STRING SEPERTI PYTHON!
	return []byte(passphraseBuilder.String()), nil
}

func evpKDF(passphrase, salt []byte) []byte {
	var key []byte
	var block []byte

	// Mengumpulkan hash hanya untuk 32 byte Key
	for len(key) < 32 {
		hasher := md5.New()
		hasher.Write(block)
		hasher.Write(passphrase)
		hasher.Write(salt)
		block = hasher.Sum(nil)
		key = append(key, block...)
	}

	return key[:32]
}

func unpadPKCS7(data []byte) ([]byte, error) {
	length := len(data)
	if length == 0 {
		return nil, errors.New("data is empty")
	}

	paddingLen := int(data[length-1])
	if paddingLen == 0 || paddingLen > length || paddingLen > aes.BlockSize {
		return nil, errors.New("invalid padding length (garbage block)")
	}

	for i := range paddingLen {
		if data[length-1-i] != byte(paddingLen) {
			return nil, errors.New("invalid padding bytes")
		}
	}

	return data[:length-paddingLen], nil
}
