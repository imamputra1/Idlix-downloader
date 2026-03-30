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
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
		DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			dialer := net.Dialer{
				Timeout:   15 * time.Second,
				KeepAlive: 30 * time.Second,
			}

			tcpConn, err := dialer.DialContext(ctx, network, addr)
			if err != nil {
				return nil, err
			}

			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				host = addr
			}

			config := &utls.Config{
				ServerName:         host,
				InsecureSkipVerify: true,
			}

			uTlsConn := utls.UClient(tcpConn, config, utls.HelloCustom)

			spec, err := utls.UTLSIdToSpec(utls.HelloChrome_Auto)
			if err != nil {
				tcpConn.Close()
				return nil, err
			}

			for _, ext := range spec.Extensions {
				if alpn, ok := ext.(*utls.ALPNExtension); ok {
					alpn.AlpnProtocols = []string{"http/1.1"}
					break
				}
			}

			if err := uTlsConn.ApplyPreset(&spec); err != nil {
				tcpConn.Close()
				return nil, err
			}

			if err := uTlsConn.HandshakeContext(ctx); err != nil {
				tcpConn.Close()
				return nil, err
			}

			return uTlsConn, nil
		},
		ForceAttemptHTTP2:     false,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   15 * time.Second,
		Jar:       jar,
	}
}
