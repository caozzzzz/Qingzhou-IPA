package http

/*
#include <stdlib.h>
char *QZNativeHTTP(const char *requestJSON);
*/
import "C"

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"
	"unsafe"
)

type nativeAuthTransport struct{}

func authenticationTransport(_ http.RoundTripper) http.RoundTripper { return nativeAuthTransport{} }

// Only login uses this adapter. The Go client still owns its cookie jar and
// allowlisted redirect validation; URLSession never follows redirects itself.
func (nativeAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := req.Context().Err(); err != nil {
		return nil, err
	}
	var body []byte
	if req.Body != nil {
		var err error
		body, err = io.ReadAll(io.LimitReader(req.Body, (1<<20)+1))
		_ = req.Body.Close()
		if err != nil {
			return nil, err
		}
		if len(body) > 1<<20 {
			return nil, errors.New("native authentication request too large")
		}
	}
	timeout := DefaultAuthenticationTimeout
	if deadline, ok := req.Context().Deadline(); ok {
		if remaining := time.Until(deadline); remaining < timeout {
			timeout = remaining
		}
	}
	if timeout <= 0 {
		return nil, errors.New("native authentication request deadline exceeded")
	}
	encoded, err := json.Marshal(struct {
		URL     string      `json:"url"`
		Method  string      `json:"method"`
		Headers http.Header `json:"headers"`
		Body    []byte      `json:"body"`
		Timeout float64     `json:"timeout"`
	}{req.URL.String(), req.Method, req.Header, body, timeout.Seconds()})
	if err != nil {
		return nil, err
	}
	input := C.CString(string(encoded))
	defer C.free(unsafe.Pointer(input))
	output := C.QZNativeHTTP(input)
	if output == nil {
		return nil, errors.New("native authentication transport returned no response")
	}
	defer C.free(unsafe.Pointer(output))
	if err := req.Context().Err(); err != nil {
		return nil, err
	}
	return decodeNativeAuthResponse([]byte(C.GoString(output)), req)
}
