package appstore

import (
	"howett.net/plist"
	"testing"
)

func TestFlatAndNestedNativeBags(t *testing.T) {
	for _, raw := range []string{`<plist><dict><key>authenticateAccount</key><string>https://auth.itunes.apple.com/auth/v1/native</string></dict></plist>`, `<plist><dict><key>urlBag</key><dict><key>authenticateAccount</key><string>https://auth.itunes.apple.com/auth/v1/native</string></dict></dict></plist>`} {
		var bag bagResult
		if _, err := plist.Unmarshal([]byte(raw), &bag); err != nil || bag.URLBag.AuthEndpoint != "https://auth.itunes.apple.com/auth/v1/native" {
			t.Fatal("native bag not decoded", err)
		}
	}
}
func TestNativeAccountIdentifierFallback(t *testing.T) {
	for _, c := range []struct {
		headers map[string]string
		want    string
	}{{map[string]string{"x-dsid": "12345"}, "12345"}, {map[string]string{"Set-Cookie": "a=b; mz_at_ssl-12345=private-token"}, "12345"}, {map[string]string{"X-Dsid": "private-token"}, ""}} {
		if got := nativeResponseDSID(c.headers); got != c.want {
			t.Fatal("invalid native account identifier")
		}
	}
}
