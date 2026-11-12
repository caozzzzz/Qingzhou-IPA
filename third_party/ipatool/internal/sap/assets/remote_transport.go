package assets

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	apphttp "github.com/majd/ipatool/v2/pkg/http"
)

func isAssetURL(u *url.URL) bool {
	return u.Scheme == "https" && u.User == nil && (u.Port() == "" || u.Port() == "443") &&
		(u.Hostname() == "updates.cdn-apple.com" || u.Hostname() == "wkms-public.apple.com")
}

func newRemoteAssetClient() *http.Client {
	transport := http.DefaultTransport
	if base, ok := transport.(*http.Transport); ok {
		direct := base.Clone()
		direct.ResponseHeaderTimeout = 8 * time.Second
		direct.TLSHandshakeTimeout = 8 * time.Second
		direct.DialContext = (&net.Dialer{Timeout: 8 * time.Second, KeepAlive: 30 * time.Second}).DialContext
		transport = direct
	}
	return &http.Client{
		Transport: &assetTransport{goHTTP: transport, native: apphttp.PublicAppleResourceRoundTripper()},
		Timeout:   25 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 || !isAssetURL(req.URL) || req.URL.Hostname() != via[0].URL.Hostname() {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
}

// All requests on this client are anonymous GETs. Remember successful fallback
// per host to avoid repeating failed Go handshakes for every archive range.
type assetTransport struct {
	goHTTP, native http.RoundTripper
	mu             sync.Mutex
	preferNative   map[string]bool
}

func (t *assetTransport) CloseIdleConnections() {
	if transport, ok := t.goHTTP.(interface{ CloseIdleConnections() }); ok {
		transport.CloseIdleConnections()
	}
}

func (t *assetTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method != http.MethodGet || req.Body != nil || !isAssetURL(req.URL) {
		return nil, errors.New("invalid Apple component request")
	}
	if t.native == nil {
		return t.goHTTP.RoundTrip(req)
	}
	host := req.URL.Hostname()
	t.mu.Lock()
	nativeFirst := t.preferNative[host]
	t.mu.Unlock()
	first, second := t.goHTTP, t.native
	if nativeFirst {
		first, second = second, first
	}
	resp, err := first.RoundTrip(req)
	if !assetTransportCanRetry(req, err) {
		return resp, err
	}
	fallback, fallbackErr := second.RoundTrip(req)
	if fallbackErr != nil {
		return nil, errors.Join(err, fallbackErr)
	}
	if fallback.StatusCode == http.StatusOK || fallback.StatusCode == http.StatusPartialContent {
		t.mu.Lock()
		if t.preferNative == nil {
			t.preferNative = make(map[string]bool)
		}
		t.preferNative[host] = !nativeFirst
		t.mu.Unlock()
	}
	return fallback, nil
}

func assetTransportCanRetry(req *http.Request, err error) bool {
	if err == nil || req.Context().Err() != nil {
		return false
	}
	var nativeErr *apphttp.NativeAuthenticationNetworkError
	if errors.As(err, &nativeErr) {
		switch nativeErr.Code {
		case -1001, -1003, -1004, -1005, -1006, -1009:
			return true
		default:
			return false
		}
	}
	var networkErr net.Error
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.As(err, &networkErr)
}
