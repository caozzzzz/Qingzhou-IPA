package emulator

import (
	"fmt"
	"net/url"
	"strings"
)

// Mirrors NSURLComponents setters while preserving separators and literal +.
// QueryEscape alone would encode the entire query as one parameter value.
func setComponentQuery(u *url.URL, value *string, encoded bool) error {
	if value == nil {
		u.RawQuery = ""
		u.ForceQuery = false
		return nil
	}
	query := *value
	if encoded {
		if _, err := url.QueryUnescape(query); err != nil {
			return err
		}
		for _, b := range []byte(query) {
			if b < 33 || b > 126 || b == '#' {
				return fmt.Errorf("invalid encoded query")
			}
		}
	} else {
		var out strings.Builder
		const allowed = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-._~!$&'()*+,;=:@/?"
		for _, b := range []byte(query) {
			if strings.IndexByte(allowed, b) >= 0 {
				out.WriteByte(b)
			} else {
				fmt.Fprintf(&out, "%%%02X", b)
			}
		}
		query = out.String()
	}
	u.RawQuery = query
	u.ForceQuery = query == ""
	return nil
}
