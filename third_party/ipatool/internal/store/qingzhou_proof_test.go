package store

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestDeviceProofResetProvisionAndReplay(t *testing.T) {
	request, _ := http.NewRequest("POST", "https://auth.itunes.apple.com/auth/v1/native", strings.NewReader("same-signed-payload"))
	request.Header.Set("X-Apple-ActionSignature", "original-signature")
	actions := []string{}
	calls := 0
	provider := func(_ context.Context, action, challenge, guid string) (map[string]string, error) {
		actions = append(actions, action)
		if action == "provision" {
			if challenge != base64.StdEncoding.EncodeToString([]byte("challenge")) {
				t.Fatal("challenge altered")
			}
			return map[string]string{"X-Apple-AMD": "fresh", "X-Apple-AMD-M": "machine"}, nil
		}
		return nil, nil
	}
	send := func(req *http.Request) (*http.Response, error) {
		calls++
		data, _ := io.ReadAll(req.Body)
		if string(data) != "same-signed-payload" || req.Header.Get("X-Apple-ActionSignature") != "original-signature" {
			t.Fatal("signed request changed")
		}
		h := http.Header{}
		if calls == 1 {
			h.Set("X-Apple-AMD-Action", "RP")
		} else if calls == 2 {
			h.Set("X-Apple-AMD-Action", "SP")
			h.Set("X-Apple-AMD-Data", base64.StdEncoding.EncodeToString([]byte("challenge")))
		} else if req.Header.Get("X-Apple-AMD") != "fresh" {
			t.Fatal("missing proof")
		}
		return &http.Response{StatusCode: 200, Header: h, Body: io.NopCloser(strings.NewReader("<plist><dict/></plist>"))}, nil
	}
	res, err := sendWithMachineProof(request, "020000000001", send, provider)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if calls != 3 || strings.Join(actions, ",") != "headers,reset,provision" {
		t.Fatal("wrong proof lifecycle")
	}
}

func TestDeviceProofRecoversFromInvalidCacheAndBodyChallenge(t *testing.T) {
	req, _ := http.NewRequest("POST", "https://auth.itunes.apple.com/auth/v1/native", strings.NewReader("signed-body"))
	actions := []string{}
	calls := 0
	res, err := sendWithMachineProof(req, "020000000001", func(request *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return &http.Response{StatusCode: 200, Header: http.Header{"X-Apple-Amd-Data": []string{"YQ=="}}, Body: io.NopCloser(strings.NewReader(`<Document><Protocol><key>customerMessage</key><string>AMD-Action:SP</string></Protocol></Document>`))}, nil
		}
		if request.Header.Get("X-Apple-AMD") != "fresh" {
			t.Fatal("proof was not refreshed")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("<plist><dict/></plist>"))}, nil
	}, func(_ context.Context, action, _, _ string) (map[string]string, error) {
		actions = append(actions, action)
		if action == "headers" {
			return nil, errors.New("invalid local identity")
		}
		if action == "provision" {
			return map[string]string{"X-Apple-AMD": "fresh", "X-Apple-AMD-M": "machine"}, nil
		}
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if calls != 2 || strings.Join(actions, ",") != "headers,reset,provision" {
		t.Fatal("cached identity did not recover")
	}
}
func TestDeviceProofNeverRunsForCDNAndRejectsInvalidChallenge(t *testing.T) {
	for _, url := range []string{"https://example.com/auth/v1/native", "https://a.cdn-apple.com/package.ipa"} {
		req, _ := http.NewRequest("POST", url, nil)
		_, err := sendWithMachineProof(req, "id", func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(""))}, nil
		}, func(context.Context, string, string, string) (map[string]string, error) {
			t.Fatal("proof escaped allowlist")
			return nil, nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	req, _ := http.NewRequest("POST", "https://auth.itunes.apple.com/auth/v1/native", nil)
	_, err := sendWithMachineProof(req, "id", func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"X-Apple-Amd-Action": []string{"SP"}, "X-Apple-Amd-Data": []string{"not-base64"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
	}, func(_ context.Context, action, _, _ string) (map[string]string, error) {
		if action != "headers" {
			t.Fatal("invalid challenge reached runtime")
		}
		return nil, nil
	})
	if err == nil {
		t.Fatal("bad challenge accepted")
	}
}
