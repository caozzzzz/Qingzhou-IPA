//go:build !ios && !qz_tci

package machine

import "time"

const sapGuestTimeout = time.Minute
