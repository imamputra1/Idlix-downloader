package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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

func padBase64(s string) string {
	if pad := len(s) % 4; pad != 0 {
		s += strings.Repeat("=", 4-pad)
	}
	return s
}

func reverseString(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

func sanitizeBase64(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'A' && r <= 'Z' || (r >= 'a' && r <= 'z') || r >= '0' && r <= '9') || r == '+' || r == '/' || r == '=' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (a AESDecrypter) DecryptURL(embedURLJSON string, outerKey string) result.Result[string] {
	var data embedData
	if err := json.Unmarshal([]byte(embedURLJSON), &data); err != nil {
		return result.Err[string](errors.New("failed to unmarshal embed_url JSON: " + err.Error()))
	}

	passphrase, err := generatePassphrase(outerKey, data.M)
	if err != nil {
		return result.Err[string](errors.New("failed to generate passphrase: " + err.Error()))
	}

	salt, err := hex.DecodeString(data.S)
	if err != nil {
		return result.Err[string](errors.New("failed to decode salt hex: " + err.Error()))
	}

	cleanCt := sanitizeBase64(padBase64(data.Ct))

	ciphertext, err := base64.StdEncoding.DecodeString(padBase64(cleanCt))
	if err != nil {
		return result.Err[string](errors.New("failed to decode ciphertext base64: " + err.Error()))
	}

	key := evpKDF(passphrase, salt)

	iv, err := hex.DecodeString(data.Iv)
	if err != nil {
		return result.Err[string](errors.New("failed to decode iv hex: " + err.Error()))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return result.Err[string](errors.New("failed to create AES cipher: " + err.Error()))
	}

	mode := cipher.NewCBCDecrypter(block, iv)
	plaintext := make([]byte, len(ciphertext))
	mode.CryptBlocks(plaintext, ciphertext)

	unpaddedPlaintext, err := pkcs7Unpad(plaintext)
	if err != nil {
		return result.Err[string](errors.New("PKCS7 unpad failed (garbage block): " + err.Error()))
	}

	finalURL := string(unpaddedPlaintext)
	finalURL = strings.ReplaceAll(finalURL, "\\", "")
	finalURL = strings.Trim(finalURL, "\"")

	return result.Ok(finalURL)
}

func generatePassphrase(outerKey string, m string) ([]byte, error) {
	var rList []string

	if strings.Contains(outerKey, "\\x") {
		for i := 2; i < len(outerKey); i += 4 {
			if i+2 <= len(outerKey) {
				rList = append(rList, outerKey[i:i+2])
			}
		}
	} else {
		for _, r := range outerKey {
			rList = append(rList, fmt.Sprintf("%02x", r))
		}
	}
	reversedM := reverseString(m)
	paddedM := padBase64(reversedM)
	cleanM := sanitizeBase64(paddedM)

	decodedMBytes, err := base64.StdEncoding.DecodeString(cleanM)
	if err != nil {
		return nil, errors.New("failed to decode base64 M: " + err.Error())
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
			passphraseBuilder.WriteString("\\x")
			passphraseBuilder.WriteString(rList[idx])
		}
	}

	return []byte(passphraseBuilder.String()), nil
}

func evpKDF(passphrase, salt []byte) []byte {
	var key []byte
	var block []byte

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

func pkcs7Unpad(data []byte) ([]byte, error) {
	length := len(data)
	if length == 0 {
		return nil, errors.New("empty data")
	}

	paddingLen := int(data[length-1])
	if paddingLen < 1 || paddingLen > aes.BlockSize || paddingLen > length {
		return nil, errors.New("invalid padding length (garbage block)")
	}

	for i := 0; i < paddingLen; i++ {
		if data[length-1-i] != byte(paddingLen) {
			return nil, errors.New("invalid padding bytes")
		}
	}

	return data[:length-paddingLen], nil
}
