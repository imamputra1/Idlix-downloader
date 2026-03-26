package crypto_test

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"io"
	"testing"

	"github.com/imamputra1/idlix-downloader/internal/infrastructure/crypto"
)

func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	padText := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(data, padText...)
}

func generateValidCiphertext(t *testing.T, plaintext, key string) string {
	t.Helper()
	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		t.Fatalf("Failed to create cipher for test generation: %v", err)
	}

	paddedText := pkcs7Pad([]byte(plaintext), aes.BlockSize)
	ciphertext := make([]byte, aes.BlockSize+len(paddedText))

	iv := ciphertext[:aes.BlockSize]
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		t.Fatalf("Failed to generate IV: %v", err)
	}

	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(ciphertext[aes.BlockSize:], paddedText)

	return base64.StdEncoding.EncodeToString(ciphertext)
}

func TestAESDecrypter_DecryptURL(t *testing.T) {
	validKey := "1234567890123456"
	wrongKey := "6543210987654321"
	invalidKeySize := "1234567890"
	cleanURL := "https://cdn.domain.com/master.m3u8"

	validCiphertext := generateValidCiphertext(t, cleanURL, validKey)

	truncatedBase64 := base64.StdEncoding.EncodeToString([]byte("1234567890"))

	tests := []struct {
		name        string
		ciphertext  string
		secretKey   string
		wantErr     bool
		expectedURL string
	}{
		{
			name:        "Scenario 1: Happy Path",
			ciphertext:  validCiphertext,
			secretKey:   validKey,
			wantErr:     false,
			expectedURL: cleanURL,
		},
		{
			name:        "Scenario 2: Key Size Boundary",
			ciphertext:  validCiphertext,
			secretKey:   invalidKeySize,
			wantErr:     true,
			expectedURL: "",
		},
		{
			name:        "Scenario 3: Malformed Base64",
			ciphertext:  "!!!invalid_base64_string!!!",
			secretKey:   validKey,
			wantErr:     true,
			expectedURL: "",
		},
		{
			name:        "Scenario 4: Truncated Payload",
			ciphertext:  truncatedBase64,
			secretKey:   validKey,
			wantErr:     true,
			expectedURL: "",
		},
		{
			name:        "Scenario 5: Padding Oracle Block (Wrong Key)",
			ciphertext:  validCiphertext,
			secretKey:   wrongKey,
			wantErr:     true,
			expectedURL: "",
		},
	}

	decrypter := crypto.NewAESDecrypter()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := decrypter.DecryptURL(tt.ciphertext, tt.secretKey)

			if res.IsErr() != tt.wantErr {
				t.Fatalf("Expected error: %v, got error: %v", tt.wantErr, res.IsErr())
			}

			if !tt.wantErr {
				url, err := res.Unwrap()
				if err != nil {
					t.Fatalf("Unexpected error from Unwrap(): %v", err)
				}
				if url != tt.expectedURL {
					t.Errorf("Expected URL: %s, got: %s", tt.expectedURL, url)
				}
			}
		})
	}
}
