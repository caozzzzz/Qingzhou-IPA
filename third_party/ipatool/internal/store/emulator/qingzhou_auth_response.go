package emulator

import "net/http"

// The login parser consumes Apple's original HTTP response. Guest response
// interpretation can reject otherwise usable account or redirect responses.
func (r *Runtime) authenticationHTTPResponse() *http.Response {
	if !r.authRequest || r.transportError != nil || r.lastResponse == nil || r.lastResponse.Body == nil {
		return nil
	}
	if r.ctx != nil && r.ctx.Err() != nil {
		return nil
	}
	return r.lastResponse
}
