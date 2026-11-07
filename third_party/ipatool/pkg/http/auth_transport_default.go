//go:build !ios

package http

import "net/http"

func AuthenticationRoundTripper(fallback http.RoundTripper) http.RoundTripper { return fallback }
