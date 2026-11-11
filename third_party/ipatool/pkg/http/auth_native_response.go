package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// NativeAuthenticationNetworkError exposes a bounded diagnostic for URLSession.
type NativeAuthenticationNetworkError struct {
	Code   int
	Domain string
}

func (e *NativeAuthenticationNetworkError) Error() string {
	return "native authentication network error (" + e.Diagnostic() + ")"
}

func (e *NativeAuthenticationNetworkError) Diagnostic() string {
	if e.Code < -100000 || e.Code > 100000 {
		return "URLSession"
	}
	switch e.Domain {
	case "", "NSURLErrorDomain":
		return fmt.Sprintf("URLSession %d", e.Code)
	case "NSPOSIXErrorDomain":
		return fmt.Sprintf("POSIX %d", e.Code)
	case "NSOSStatusErrorDomain":
		return fmt.Sprintf("OSStatus %d", e.Code)
	case "kCFErrorDomainCFNetwork":
		return fmt.Sprintf("CFNetwork %d", e.Code)
	default:
		return "URLSession"
	}
}
func (e *NativeAuthenticationNetworkError) Timeout() bool {
	return (e.Domain == "" || e.Domain == "NSURLErrorDomain") && e.Code == -1001
}
func (e *NativeAuthenticationNetworkError) Temporary() bool { return false }

// Decode only inside the trusted bridge; never include wire data in errors.
func decodeNativeAuthResponse(data []byte, req *http.Request) (*http.Response, error) {
	var result struct {
		Status    int         `json:"status"`
		Headers   http.Header `json:"headers"`
		Body      []byte      `json:"body"`
		ErrorCode int         `json:"errorCode"`
		Domain    string      `json:"errorDomain"`
		Cookies   []struct {
			Name     string
			Value    string
			Domain   string
			Path     string
			Expires  float64
			Secure   bool
			HTTPOnly bool
		} `json:"cookies"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, errors.New("invalid native authentication response")
	}
	if result.ErrorCode != 0 {
		return nil, &NativeAuthenticationNetworkError{Code: result.ErrorCode, Domain: result.Domain}
	}
	if result.Status < 100 || result.Status > 599 {
		return nil, errors.New("invalid native authentication HTTP status")
	}
	canonical := make(http.Header)
	for name, values := range result.Headers {
		for _, value := range values {
			canonical.Add(name, value)
		}
	}
	result.Headers = canonical
	// URLSession transparently decompresses these encodings. Do not hand the
	// original compression headers to the framework or it may decode twice.
	uncompressed := false
	switch strings.ToLower(result.Headers.Get("Content-Encoding")) {
	case "gzip", "deflate", "br":
		result.Headers.Del("Content-Encoding")
		result.Headers.Set("Content-Length", strconv.Itoa(len(result.Body)))
		uncompressed = true
	}
	if len(result.Cookies) > 0 {
		result.Headers.Del("Set-Cookie")
		for _, c := range result.Cookies {
			cookie := &http.Cookie{Name: c.Name, Value: c.Value, Domain: c.Domain, Path: c.Path, Secure: c.Secure, HttpOnly: c.HTTPOnly}
			if c.Expires > 0 {
				cookie.Expires = time.Unix(int64(c.Expires), 0)
			}
			if value := cookie.String(); value != "" {
				result.Headers.Add("Set-Cookie", value)
			}
		}
	}
	return &http.Response{StatusCode: result.Status, Status: fmt.Sprintf("%d %s", result.Status, http.StatusText(result.Status)),
		Header: result.Headers, Body: io.NopCloser(bytes.NewReader(result.Body)), ContentLength: int64(len(result.Body)),
		Request: req, Proto: "URLSession", Uncompressed: uncompressed}, nil
}
