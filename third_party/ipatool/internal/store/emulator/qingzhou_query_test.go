package emulator

import (
	"net/url"
	"testing"
)

func TestComponentQuerySetters(t *testing.T) {
	for _, c := range []struct {
		value   string
		encoded bool
		want    string
	}{
		{"bundleId=com.example.app&token=a+b=c", false, "https://example.test/path?bundleId=com.example.app&token=a+b=c"},
		{"name=中文 app#1&rate=100%", false, "https://example.test/path?name=%E4%B8%AD%E6%96%87%20app%231&rate=100%25"},
		{"token=a%2Bb%3D&next=%2Fquery", true, "https://example.test/path?token=a%2Bb%3D&next=%2Fquery"},
		{"", false, "https://example.test/path?"},
	} {
		u, _ := url.Parse("https://example.test/path?previous=1")
		if err := setComponentQuery(u, &c.value, c.encoded); err != nil {
			t.Fatal(err)
		}
		if u.String() != c.want {
			t.Fatalf("got %s, want %s", u.String(), c.want)
		}
		if err := setComponentQuery(u, nil, c.encoded); err != nil || u.String() != "https://example.test/path" {
			t.Fatal("nil did not clear query")
		}
	}
	for _, value := range []string{"bad=%xx", "query=space here", "query=#fragment"} {
		u, _ := url.Parse("https://example.test/path?original=1")
		if setComponentQuery(u, &value, true) == nil || u.RawQuery != "original=1" {
			t.Fatal("invalid encoded query accepted or partially applied")
		}
	}
}
