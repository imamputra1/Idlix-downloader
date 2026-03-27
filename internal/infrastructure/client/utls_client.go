package client

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/cookiejar"
	"time"

	utls "github.com/refraction-networking/utls"
)

func NewAntiBotClient() *http.Client {
	jar, err := cookiejar.New(nil)
	if err != nil {
		panic("Gagal menginisialisasi CookieJar: " + err.Error())
	}

	transport := &http.Transport{
		// Ini digunakan oleh Go jika sewaktu-waktu ia fallback dari uTLS
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
		DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			dialer := net.Dialer{
				Timeout:   15 * time.Second,
				KeepAlive: 30 * time.Second,
			}

			// 1. Inisiasi koneksi TCP murni
			tcpConn, err := dialer.DialContext(ctx, network, addr)
			if err != nil {
				return nil, err
			}

			// 2. Ekstraksi Hostname untuk Server Name Indication (SNI)
			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				host = addr
			}

			// SURGERY FIX: Tambahkan InsecureSkipVerify di utls.Config agar sinkron
			config := &utls.Config{
				ServerName:         host,
				InsecureSkipVerify: true, // <- KUNCI PENTING agar tidak diblokir cert bodong
			}

			// 3. Buat uTLS Client dengan mode Custom
			uTlsConn := utls.UClient(tcpConn, config, utls.HelloCustom)

			// 4. Ambil spesifikasi Chrome terbaru
			spec, err := utls.UTLSIdToSpec(utls.HelloChrome_Auto)
			if err != nil {
				tcpConn.Close()
				return nil, err
			}

			// 5. Modifikasi ALPN untuk mematikan HTTP/2 secara paksa (Downgrade ke HTTP/1.1)
			// Ini ampuh menembus WAF karena WAF sering mencegat anomali frame HTTP/2
			for _, ext := range spec.Extensions {
				if alpn, ok := ext.(*utls.ALPNExtension); ok {
					alpn.AlpnProtocols = []string{"http/1.1"}
					break
				}
			}

			// 6. Terapkan spesifikasi yang sudah dimodifikasi
			if err := uTlsConn.ApplyPreset(&spec); err != nil {
				tcpConn.Close()
				return nil, err
			}

			// 7. Lakukan Handshake dengan menyertakan Context
			if err := uTlsConn.HandshakeContext(ctx); err != nil {
				tcpConn.Close()
				return nil, err
			}

			return uTlsConn, nil
		},
		ForceAttemptHTTP2:     false, // Sinkron dengan modifikasi ALPN kita di atas
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   15 * time.Second,
		Jar:       jar, // Stateful Memory (Penyimpan Cookie) aktif
	}
}
