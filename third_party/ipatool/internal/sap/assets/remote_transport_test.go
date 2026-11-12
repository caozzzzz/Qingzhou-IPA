package assets

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

type fixtureRoundTripper func(*http.Request) (*http.Response, error)

func (f fixtureRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestAppleAssetSwitchesToSystemAfterHandshakeEOF(t *testing.T) {
	var goCalls, nativeCalls int
	transport := &assetTransport{
		goHTTP: fixtureRoundTripper(func(r *http.Request) (*http.Response, error) {
			goCalls++
			return nil, io.EOF
		}),
		native: fixtureRoundTripper(func(r *http.Request) (*http.Response, error) {
			nativeCalls++
			if r.Header.Get("Range") != "bytes=0-0" {
				t.Fatal("fallback lost the range")
			}
			return &http.Response{StatusCode: http.StatusPartialContent, Body: http.NoBody}, nil
		}),
	}
	req, _ := http.NewRequest(http.MethodGet, macOSRestoreURL, nil)
	req.Header.Set("Range", "bytes=0-0")
	for range 2 {
		resp, err := transport.RoundTrip(req)
		if err != nil || resp.StatusCode != http.StatusPartialContent {
			t.Fatalf("fallback failed: %v", err)
		}
		resp.Body.Close()
	}
	if goCalls != 1 || nativeCalls != 2 {
		t.Fatalf("unexpected route calls: %d Go, %d system", goCalls, nativeCalls)
	}
	req.URL.Host = "example.test"
	if _, err := transport.RoundTrip(req); err == nil || nativeCalls != 2 {
		t.Fatal("request escaped the public Apple host allowlist")
	}
}

func TestRestoreRangeResumesTruncatedBodyAndBoundsReads(t *testing.T) {
	contents := bytes.Repeat([]byte{7, 1, 4}, maxRemoteRange/3+9)
	truncated, requests := false, 0
	remote := remoteFile{ctx: context.Background(), url: macOSRestoreURL, size: int64(len(contents)),
		client: &http.Client{Transport: fixtureRoundTripper(func(req *http.Request) (*http.Response, error) {
			requests++
			var start, end int
			if _, err := fmt.Sscanf(req.Header.Get("Range"), "bytes=%d-%d", &start, &end); err != nil ||
				end-start+1 > maxRemoteRange || req.Header.Get("Accept-Encoding") != "identity" {
				t.Fatal("incorrect range or encoding")
			}
			data := contents[start : end+1]
			if !truncated {
				truncated = true
				data = data[:13]
			} else if requests == 2 && start != 13 {
				t.Fatal("range did not resume from the first missing byte")
			}
			h := http.Header{"Content-Range": {fmt.Sprintf("bytes %d-%d/%d", start, end, len(contents))}}
			return &http.Response{StatusCode: http.StatusPartialContent, Header: h, Body: io.NopCloser(bytes.NewReader(data))}, nil
		})},
	}
	data := make([]byte, len(contents))
	if n, err := remote.ReadAt(data, 0); n != len(contents) || err != nil || !bytes.Equal(contents, data) || requests != 3 {
		t.Fatalf("restore read: %d bytes, %d requests, error %v", n, requests, err)
	}
}

func TestSAPImportRejectsIncompleteComponents(t *testing.T) {
	folder := filepath.Join(t.TempDir(), "SAP-Components")
	if _, found, err := readImport(folder); found || err != nil {
		t.Fatalf("missing optional folder: found=%v err=%v", found, err)
	}
	if err := os.Mkdir(folder, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "AppleMediaServices"), []byte("invalid"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, found, err := readImport(folder); found || err == nil {
		t.Fatalf("invalid component accepted: found=%v err=%v", found, err)
	}
}
