package http

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestNativeResponsePreservesHeadersBodyAndCookies(t *testing.T) {
	req, _ := http.NewRequest("POST", "https://buy.itunes.apple.com"+appStoreAuthPath+"/", nil)
	wire := []byte(`{"status":302,"headers":{"location":["https://p7-buy.itunes.apple.com/WebObjects/MZFinance.woa/wa/authenticate/"],"set-cookie":["combined=ignore"]},"body":"aGVsbG8=","cookies":[{"Name":"first","Value":"one","Domain":"buy.itunes.apple.com","Path":"/","HTTPOnly":true},{"Name":"second","Value":"two","Domain":"buy.itunes.apple.com","Path":"/","Secure":true}]}`)
	res, err := decodeNativeAuthResponse(wire, req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.Header.Get("Location") == "" || string(body) != "hello" || res.Request != req || res.Proto != "URLSession" {
		t.Fatal("response information lost")
	}
	cookies := res.Cookies()
	if len(cookies) != 2 || !cookies[0].HttpOnly || !cookies[1].Secure {
		t.Fatal("cookie properties lost")
	}
	sut := &client[struct{}]{authentication: true}
	res.Body = io.NopCloser(strings.NewReader("hello"))
	parsed, err := sut.handleXMLResponse(res)
	if err != nil || parsed.StatusCode != 302 {
		t.Fatal("redirect not passed to validator")
	}
}
func TestNativeMissingLocationRetainsSafeDiagnostics(t *testing.T) {
	req, _ := http.NewRequest("POST", "https://buy.itunes.apple.com"+appStoreAuthPath+"/", nil)
	res, err := decodeNativeAuthResponse([]byte(`{"status":302,"headers":{"content-type":["text/html"]},"body":"c2VjcmV0"}`), req)
	if err != nil {
		t.Fatal(err)
	}
	sut := &client[struct{}]{authentication: true}
	_, err = sut.handleXMLResponse(res)
	failure, ok := err.(*UnexpectedResponseError)
	if !ok || failure.Reason != "authentication redirect is missing Location" || failure.Method != "POST" || failure.AuthPath != "AUTH_SLASH" || failure.Protocol != "URLSession" || failure.BodyLength != 6 || strings.Contains(err.Error(), "secret") {
		t.Fatal("unsafe or incomplete diagnostics")
	}
}
func TestNativeMalformedResponseDoesNotEchoSecrets(t *testing.T) {
	for _, wire := range []string{`secret-password`, `{"errorCode":-1202,"body":"secret-password"}`, `{"status":0}`} {
		_, err := decodeNativeAuthResponse([]byte(wire), nil)
		if err == nil || strings.Contains(err.Error(), "secret-password") {
			t.Fatal("unsafe native error")
		}
	}
}
