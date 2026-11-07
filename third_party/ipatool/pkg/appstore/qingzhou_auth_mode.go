package appstore

import "sync/atomic"

// Authentication endpoint path modes used for on-device comparison. The
// default keeps the trailing-slash normalization applied to the bag URL.
const (
	AuthenticationPathSlash = "slash"
	AuthenticationPathBare  = "bare"
)

var authenticationPathBare atomic.Bool

// SetAuthenticationPathMode selects the authentication endpoint path for
// subsequent logins. Unknown values select the default trailing-slash path.
func SetAuthenticationPathMode(mode string) {
	authenticationPathBare.Store(mode == AuthenticationPathBare)
}

// AuthenticationPathMode reports the currently selected endpoint path mode.
func AuthenticationPathMode() string {
	if authenticationPathBare.Load() {
		return AuthenticationPathBare
	}

	return AuthenticationPathSlash
}
