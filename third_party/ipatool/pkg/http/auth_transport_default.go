//go:build !ios

package http

import "net/http"

func authenticationTransport(fallback http.RoundTripper) http.RoundTripper { return fallback }
