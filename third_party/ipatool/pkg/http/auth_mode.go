package http

import "sync/atomic"

// Authentication transport modes. "native" uses the platform transport where
// one exists (URLSession on iOS); "go" always uses Go's isolated net/http
// transport. Other platforms ignore the setting and always use net/http.
const (
	AuthenticationTransportNative = "native"
	AuthenticationTransportGo     = "go"
)

var authenticationTransportGo atomic.Bool

// SetAuthenticationTransport selects the transport for subsequent login
// requests. Unknown values select the native transport.
func SetAuthenticationTransport(mode string) {
	authenticationTransportGo.Store(mode == AuthenticationTransportGo)
}

// AuthenticationTransport reports the currently selected login transport.
func AuthenticationTransport() string {
	if authenticationTransportGo.Load() {
		return AuthenticationTransportGo
	}

	return AuthenticationTransportNative
}
