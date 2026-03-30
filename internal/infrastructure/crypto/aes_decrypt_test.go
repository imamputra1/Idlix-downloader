package crypto_test

import (
	"strings"
	"testing"

	"github.com/imamputra1/idlix-downloader/internal/infrastructure/crypto"
)

func TestAESDecrypter_DecryptURL(t *testing.T) {
	decrypter := crypto.NewAESDecrypter()

	tests := []struct {
		name        string
		payloadJSON string
		outerKey    string
		wantErr     bool
		errContains string
	}{
		{
			name:        "Scenario 1: Invalid JSON Payload",
			payloadJSON: "bukan-json-yang-valid",
			outerKey:    "dummy-key",
			wantErr:     true,
			errContains: "failed to unmarshal",
		},
		{
			name:        "Scenario 2: Missing or Invalid CT (Base64)",
			payloadJSON: `{"ct": "!!!invalid_base64!!!", "iv": "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6", "s": "e5f6a7b8c9d0e1f2", "m": "ywXM"}`,
			outerKey:    `\x4a\x7a\x6b\x4d`,
			wantErr:     true,
			errContains: "failed to decode ct base64",
		},
		{
			name:        "Scenario 3: Invalid Salt Hex",
			payloadJSON: `{"ct": "AAAA", "iv": "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6", "s": "bukan_hex", "m": "ywXM"}`,
			outerKey:    `\x4a\x7a\x6b\x4d`,
			wantErr:     true,
			errContains: "failed to decode salt hex",
		},
		{
			name:        "Scenario 4: Invalid IV Hex",
			payloadJSON: `{"ct": "AAAA", "iv": "bukan_hex", "s": "e5f6a7b8c9d0e1f2", "m": "ywXM"}`,
			outerKey:    `\x4a\x7a\x6b\x4d`,
			wantErr:     true,
			errContains: "failed to decode IV hex",
		},

		{
			name:        "Scenario 5: Happy Path (Real Data Aggregator)",
			payloadJSON: `{"ct":"BFArUsXSnuwqR9T\/UBHR2Zlw3FwLUXi1n+qv2BXkeKsySzqYd\/2KGPSHLIz86Xq8tuZtHV8\/9Vf8hb8fdCePbNFu0mczdtIxZ7iZTGUFSJs=","iv":"b0bb3661bd5d7f5cc6d0d6344177cd78","s":"921d2c358aea887d","m":"ANzwnM8BjM8hzM8djM8FzM8VjM8hTM8hjM8dDf5MDfyQDf0EDfxwHMxwXNzwXO8ZTM8hDfwQDf3EDfwMDfzIDf2w3M8JzM8RjM8lTM8ZjM8VTM8NDN8RDfxEDf2MDf5IDf1wnMxwHM8JjM8NzM8NTM8FDN8FjM8dzM"}`,
			outerKey:    `\x59\x56\x4d\x33\x4d\x4f\x59\x67\x54\x6c\x6d\x34\x32\x78\x6c\x67\x46\x32\x5a\x4d\x57\x7a\x6a\x6a\x6a\x44\x30\x31\x35\x47\x55\x4d\x49\x5a\x3d\x4e\x45\x4d\x4d\x4d\x4d\x59\x4e\x7a`,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := decrypter.DecryptURL(tt.payloadJSON, tt.outerKey)

			if tt.wantErr {
				if !res.IsErr() {
					val, _ := res.Unwrap()
					t.Fatalf("Expected an error but got success with URL: %s", val)
				}

				_, err := res.Unwrap()
				if err != nil && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("Expected error to contain '%s', but got: '%v'", tt.errContains, err.Error())
				}
			} else {
				if res.IsErr() {
					_, err := res.Unwrap()
					t.Fatalf("Expected success but got error: %v", err)
				}

				url, _ := res.Unwrap()
				if url == "" {
					t.Errorf("Expected valid URL, got empty string")
				} else {
					t.Logf("Decrypted URL: %s", url)
				}
			}
		})
	}
}
