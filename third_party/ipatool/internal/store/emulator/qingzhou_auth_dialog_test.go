package emulator

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestAuthenticationDialogFallbackIsNarrowAndPreservesResponse(t *testing.T) {
	body := `<?xml version="1.0"?><plist version="1.0"><dict><key>dialog</key><dict><key>message</key><string>fixture-private-message</string></dict></dict></plist>`
	for _, c := range []struct {
		auth          bool
		failure, body string
		want          bool
	}{
		{true, "unsupported import AMSError", body, true},
		{false, "unsupported import AMSError", body, false},
		{true, "unsupported import OtherSymbol", body, false},
		{true, "unsupported import AMSError", "<html>private</html>", false},
		{true, "unsupported import AMSError", `<plist><dict><key>unrelated</key><string>private</string></dict></plist>`, false},
	} {
		original := &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(c.body))}
		r := &Runtime{authRequest: c.auth, lastResponse: original}
		res, ok := r.recoverAuthenticationDialog(errors.New(c.failure))
		if ok != c.want {
			t.Fatal("unexpected fallback decision")
		}
		if ok {
			data, _ := io.ReadAll(res.Body)
			if res != original || string(data) != c.body {
				t.Fatal("original response was altered")
			}
		}
	}
}
