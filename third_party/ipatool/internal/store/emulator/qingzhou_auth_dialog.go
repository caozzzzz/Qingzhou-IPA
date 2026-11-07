package emulator

import (
	"bytes"
	"howett.net/plist"
	"io"
	"net/http"
	"strings"
)

// Upstream #634: the guest can fail importing AMSError while interpreting an
// actual Apple account dialog. Preserve only a recognized original plist for
// the existing strict login parser. This never creates credentials or success.
func (r *Runtime) recoverAuthenticationDialog(err error) (*http.Response, bool) {
	if !r.authRequest || r.lastResponse == nil || r.lastResponse.Body == nil || !strings.Contains(err.Error(), "unsupported import AMSError") {
		return nil, false
	}
	data, readErr := io.ReadAll(io.LimitReader(r.lastResponse.Body, (2<<20)+1))
	_ = r.lastResponse.Body.Close()
	r.lastResponse.Body = io.NopCloser(bytes.NewReader(data))
	if readErr != nil || len(data) > 2<<20 {
		return nil, false
	}
	var values map[string]any
	if _, e := plist.Unmarshal(data, &values); e != nil {
		return nil, false
	}
	recognized := false
	if dialog, ok := values["dialog"].(map[string]any); ok && len(dialog) > 0 {
		recognized = true
	}
	for _, key := range []string{"failureType", "customerMessage"} {
		if value, ok := values[key].(string); ok && value != "" {
			recognized = true
		}
	}
	if !recognized {
		return nil, false
	}
	return r.lastResponse, true
}
