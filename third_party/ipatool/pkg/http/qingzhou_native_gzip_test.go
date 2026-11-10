package http

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func TestNativeDecodedBodyCannotBeDecompressedTwice(t *testing.T) {
	body := []byte("<plist><dict/></plist>")
	data, _ := json.Marshal(map[string]any{"status": 200, "body": body, "headers": http.Header{"Content-Encoding": []string{"gzip"}, "Content-Length": []string{"999"}}})
	request, _ := http.NewRequest("POST", "https://auth.itunes.apple.com/auth/v1/native", nil)
	response, err := decodeNativeAuthResponse(data, request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	got, _ := io.ReadAll(response.Body)
	if string(got) != string(body) || response.Header.Get("Content-Encoding") != "" || !response.Uncompressed || response.ContentLength != int64(len(body)) {
		t.Fatal("URLSession-decoded body kept stale compression metadata")
	}
}
