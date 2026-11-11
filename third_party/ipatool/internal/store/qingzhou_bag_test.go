package store

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	apphttp "github.com/majd/ipatool/v2/pkg/http"
)

type bagTestTransport func(*http.Request) (*http.Response, error)

func (f bagTestTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

const testAuthenticationBagURL = "https://init.itunes.apple.com/bag.xml?guid=020000000045"

func TestAuthenticationBagRetriesOnlyPublicConfiguration(t *testing.T) {
	const plist = `<?xml version="1.0"?><plist version="1.0"><dict><key>urlBag</key><dict><key>sign-sap-version</key><string>200</string></dict></dict></plist>`
	attempts := 0
	checkRequest := func(req *http.Request) {
		t.Helper()
		attempts++
		if req.Method != http.MethodGet || req.URL.Host != "init.itunes.apple.com" ||
			req.URL.Query().Get("ix") != "6" || req.URL.Query().Get("guid") != "020000000045" ||
			req.Body != nil || req.Header.Get("Cookie") != "" || req.Header.Get("Authorization") != "" {
			t.Fatal("unexpected public configuration request")
		}
	}
	primary := bagTestTransport(func(req *http.Request) (*http.Response, error) {
		checkRequest(req)
		return nil, io.EOF
	})
	fallback := bagTestTransport(func(req *http.Request) (*http.Response, error) {
		checkRequest(req)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(plist))}, nil
	})
	response, values, err := readAuthenticationBag(context.Background(), testAuthenticationBagURL, []bagTransport{
		{apphttp.AuthenticationTransportGo, primary},
		{apphttp.AuthenticationTransportNative, fallback},
	})
	if err != nil || attempts != 2 || values["urlBag"] == nil {
		t.Fatalf("public configuration fallback failed: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || string(body) != plist {
		t.Fatalf("Apple configuration body was changed: %v", err)
	}
}

func TestAuthenticationBagStopsAtRateLimitAndNeverFollowsRedirect(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusFound} {
		attempts := 0
		transport := bagTestTransport(func(req *http.Request) (*http.Response, error) {
			attempts++
			if req.URL.Host != "init.itunes.apple.com" {
				t.Fatal("followed redirect")
			}
			return &http.Response{
				StatusCode: status, Header: http.Header{"Location": {"https://example.invalid/"}},
				Body: io.NopCloser(strings.NewReader("")),
			}, nil
		})
		_, _, err := readAuthenticationBag(context.Background(), testAuthenticationBagURL, []bagTransport{
			{apphttp.AuthenticationTransportGo, transport}, {apphttp.AuthenticationTransportNative, transport},
		})
		var failure *apphttp.AuthenticationBagError
		if !errors.As(err, &failure) || len(failure.Attempts) != attempts || failure.Attempts[0].StatusCode != status {
			t.Fatalf("unexpected public configuration diagnostic: %v", err)
		}
		if status == http.StatusTooManyRequests && attempts != 1 {
			t.Fatal("repeated request after rate limit")
		}
	}
}
