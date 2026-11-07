//go:build ios || qz_tci

package machine

import "time"

// Interpretation trades speed for running without executable-memory permissions.
const sapGuestTimeout = 5 * time.Minute
