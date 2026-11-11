package emulator

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestAuthenticationHTTPResponsePreservesAppleReply(t *testing.T) {
	const body = "<html>redirect response</html>"
	original := &http.Response{
		StatusCode: http.StatusFound,
		Header:     http.Header{"Location": {"https://auth.itunes.apple.com/auth/v1/native"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
	r := &Runtime{authRequest: true, lastResponse: original}
	res := r.authenticationHTTPResponse()
	if res != original {
		t.Fatal("expected original Apple response")
	}
	data, err := io.ReadAll(res.Body)
	if err != nil || string(data) != body {
		t.Fatalf("response body changed: %v", err)
	}

	r.authRequest = false
	if r.authenticationHTTPResponse() != nil {
		t.Fatal("other Store operations must use the guest parser")
	}
	r.authRequest = true
	r.transportError = io.ErrUnexpectedEOF
	if r.authenticationHTTPResponse() != nil {
		t.Fatal("must not reuse a response after a transport failure")
	}
	r.transportError = nil
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r.ctx = ctx
	if r.authenticationHTTPResponse() != nil {
		t.Fatal("must not reuse a response after cancellation")
	}
}
