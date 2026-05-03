// Package outbound builds HTTP clients for all external egress traffic.
package outbound

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/http2"
	xproxy "golang.org/x/net/proxy"

	"github.com/kleinai/backend/pkg/proxyx"
)

const (
	ModeStandard = "standard"
	ModeUTLS     = "utls"

	ProfileChrome = "chrome"
)

// Options describes one outbound client.
type Options struct {
	ProxyURL string
	Timeout  time.Duration
	Mode     string
	Profile  string
}

// NewClient creates a client with a consistent proxy and TLS stack.
func NewClient(opt Options) (*http.Client, error) {
	if opt.Timeout <= 0 {
		opt.Timeout = 30 * time.Second
	}
	mode := strings.ToLower(strings.TrimSpace(opt.Mode))
	if mode == "" {
		mode = ModeUTLS
	}
	profile := strings.ToLower(strings.TrimSpace(opt.Profile))
	if profile == "" {
		profile = ProfileChrome
	}

	pu, err := proxyx.Parse(opt.ProxyURL)
	if err != nil {
		return nil, err
	}
	if mode == ModeStandard {
		return proxyx.BuildClient(pu, opt.Timeout)
	}
	if mode != ModeUTLS {
		return nil, fmt.Errorf("unsupported outbound mode %q", opt.Mode)
	}
	return &http.Client{
		Transport: &utlsTransport{
			proxyURL: pu,
			profile:  profile,
			timeout:  opt.Timeout,
		},
		Timeout: opt.Timeout,
	}, nil
}

type utlsTransport struct {
	proxyURL *url.URL
	profile  string
	timeout  time.Duration
}

func (t *utlsTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL == nil {
		return nil, fmt.Errorf("missing request URL")
	}
	if req.URL.Scheme != "https" {
		return t.standardRoundTrip(req)
	}

	conn, err := t.dialTarget(req.Context(), req.URL)
	if err != nil {
		return nil, err
	}
	closeOnErr := true
	defer func() {
		if closeOnErr {
			_ = conn.Close()
		}
	}()

	serverName := req.URL.Hostname()
	tlsConn := utls.UClient(conn, &utls.Config{
		ServerName:         serverName,
		InsecureSkipVerify: true, //nolint:gosec // Commercial proxies may MITM CONNECT targets.
		MinVersion:         tls.VersionTLS12,
		NextProtos:         []string{"h2", "http/1.1"},
	}, t.clientHelloID())
	if err := tlsConn.HandshakeContext(req.Context()); err != nil {
		return nil, fmt.Errorf("tls handshake to %s failed: %w", req.URL.Host, err)
	}
	if tlsConn.ConnectionState().NegotiatedProtocol == "h2" {
		h2Transport := &http2.Transport{}
		cc, err := h2Transport.NewClientConn(tlsConn)
		if err != nil {
			return nil, fmt.Errorf("create http2 client failed: %w", err)
		}
		resp, err := cc.RoundTrip(req)
		if err != nil {
			return nil, fmt.Errorf("http2 request failed: %w", err)
		}
		resp.Body = &connReadCloser{Reader: resp.Body, closer: tlsConn}
		closeOnErr = false
		return resp, nil
	}

	if err := writeRequest(tlsConn, req); err != nil {
		return nil, fmt.Errorf("write request failed: %w", err)
	}
	br := bufio.NewReader(tlsConn)
	resp, err := http.ReadResponse(br, req)
	if err != nil {
		return nil, fmt.Errorf("read response failed: %w", err)
	}
	resp.Body = &connReadCloser{Reader: resp.Body, closer: tlsConn}
	closeOnErr = false
	return resp, nil
}

func (t *utlsTransport) standardRoundTrip(req *http.Request) (*http.Response, error) {
	client, err := proxyx.BuildClient(t.proxyURL, t.timeout)
	if err != nil {
		return nil, err
	}
	return client.Transport.RoundTrip(req)
}

func (t *utlsTransport) dialTarget(ctx context.Context, target *url.URL) (net.Conn, error) {
	addr := canonicalAddr(target)
	if t.proxyURL == nil {
		return (&net.Dialer{Timeout: t.timeout, KeepAlive: 30 * time.Second}).DialContext(ctx, "tcp", addr)
	}
	switch strings.ToLower(t.proxyURL.Scheme) {
	case "http", "https":
		return t.dialHTTPProxy(ctx, addr)
	case "socks5", "socks5h":
		return t.dialSOCKS5(ctx, addr)
	default:
		return nil, fmt.Errorf("unsupported proxy scheme %q", t.proxyURL.Scheme)
	}
}

func (t *utlsTransport) dialHTTPProxy(ctx context.Context, targetAddr string) (net.Conn, error) {
	proxyAddr := t.proxyURL.Host
	conn, err := (&net.Dialer{Timeout: t.timeout, KeepAlive: 30 * time.Second}).DialContext(ctx, "tcp", proxyAddr)
	if err != nil {
		return nil, fmt.Errorf("connect proxy %s failed: %w", proxyAddr, err)
	}
	closeOnErr := true
	defer func() {
		if closeOnErr {
			_ = conn.Close()
		}
	}()

	var proxyConn net.Conn = conn
	if strings.EqualFold(t.proxyURL.Scheme, "https") {
		host := t.proxyURL.Hostname()
		tlsProxy := tls.Client(conn, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
		if err := tlsProxy.HandshakeContext(ctx); err != nil {
			return nil, fmt.Errorf("tls handshake to proxy %s failed: %w", proxyAddr, err)
		}
		proxyConn = tlsProxy
	}

	connectReq := "CONNECT " + targetAddr + " HTTP/1.1\r\nHost: " + targetAddr + "\r\nProxy-Connection: Keep-Alive\r\n"
	if t.proxyURL.User != nil {
		pw, _ := t.proxyURL.User.Password()
		token := base64.StdEncoding.EncodeToString([]byte(t.proxyURL.User.Username() + ":" + pw))
		connectReq += "Proxy-Authorization: Basic " + token + "\r\n"
	}
	connectReq += "\r\n"
	if _, err := io.WriteString(proxyConn, connectReq); err != nil {
		return nil, fmt.Errorf("write CONNECT to proxy failed: %w", err)
	}
	br := bufio.NewReader(proxyConn)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodConnect})
	if err != nil {
		return nil, fmt.Errorf("read CONNECT response from proxy failed: %w", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("proxy CONNECT %s returned HTTP %d", targetAddr, resp.StatusCode)
	}
	closeOnErr = false
	return proxyConn, nil
}

func (t *utlsTransport) dialSOCKS5(ctx context.Context, targetAddr string) (net.Conn, error) {
	var auth *xproxy.Auth
	if t.proxyURL.User != nil {
		pw, _ := t.proxyURL.User.Password()
		auth = &xproxy.Auth{User: t.proxyURL.User.Username(), Password: pw}
	}
	dialer, err := xproxy.SOCKS5("tcp", t.proxyURL.Host, auth, &net.Dialer{
		Timeout:   t.timeout,
		KeepAlive: 30 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("build socks5 dialer: %w", err)
	}
	ctxDialer, ok := dialer.(xproxy.ContextDialer)
	if !ok {
		return nil, fmt.Errorf("socks5 dialer does not support context")
	}
	conn, err := ctxDialer.DialContext(ctx, "tcp", targetAddr)
	if err != nil {
		return nil, fmt.Errorf("socks5 connect %s failed: %w", targetAddr, err)
	}
	return conn, nil
}

func (t *utlsTransport) clientHelloID() utls.ClientHelloID {
	switch t.profile {
	case ProfileChrome, "":
		return utls.HelloChrome_133
	default:
		return utls.HelloChrome_133
	}
}

func writeRequest(w io.Writer, req *http.Request) error {
	out := req.Clone(req.Context())
	out.RequestURI = ""
	out.URL = cloneURL(req.URL)
	out.Header = req.Header.Clone()
	if out.Header.Get("Host") == "" && req.Host != "" {
		out.Host = req.Host
	}
	return out.Write(w)
}

func cloneURL(u *url.URL) *url.URL {
	if u == nil {
		return nil
	}
	cp := *u
	return &cp
}

func canonicalAddr(u *url.URL) string {
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	return net.JoinHostPort(host, port)
}

type connReadCloser struct {
	io.Reader
	closer io.Closer
}

func (c *connReadCloser) Close() error {
	return c.closer.Close()
}
