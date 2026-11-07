package main

import (
	"errors"
	"github.com/majd/ipatool/v2/pkg/appstore"
	storehttp "github.com/majd/ipatool/v2/pkg/http"
	"io"
	"strings"
	"testing"
)

func TestAuthenticationEnvironmentErrorsAreClassifiedWithoutDetails(t *testing.T) {
	cases := map[string]string{
		"cache Apple SAP assets: private-path-secret":         "auth_cache",
		"load Apple SAP assets: private-network-secret":       "auth_assets",
		"failed to get bag: private-response-secret":          "auth_bag",
		"create Unicorn engine: private-runtime-secret":       "auth_engine",
		"execute SAP guest function: private-register-secret": "auth_engine",
	}
	for detail, want := range cases {
		code, message := authenticationError(errors.New(detail))
		if code != want || strings.Contains(message, "private-") {
			t.Fatalf("unexpected public error %s: %s", code, message)
		}
	}
}

func TestAuthenticationHTTPDiagnosticsNeverExposeResponseDetails(t *testing.T) {
	code, message := authenticationError(&storehttp.TransportError{Err: io.EOF})
	if code != "auth_transport" || !strings.Contains(message, "EOF") {
		t.Fatal("transport classification missing")
	}
	for _, status := range []int{401, 403, 404, 429, 500} {
		err := &storehttp.UnexpectedResponseError{StatusCode: status, Snippet: "private-password-token", CorrelationID: "private-id"}
		code, message := authenticationError(err)
		if code == "apple" || strings.Contains(message, "private-") {
			t.Fatalf("unsafe or missing HTTP diagnostic: %s", message)
		}
	}
	code, message = authenticationError(&appstore.AuthenticationFailure{StatusCode: 200, FailureType: "-5000"})
	if code != "credentials" || !strings.Contains(message, "-5000") {
		t.Fatal("credential code missing")
	}
}

func TestMissingRedirectDiagnosticsAreAllowlisted(t *testing.T) {
	err := &storehttp.UnexpectedResponseError{StatusCode: 302, Reason: "authentication redirect is missing Location", Protocol: "URLSession", Method: "POST", AuthPath: "AUTH_SLASH", ContentType: "text/html; private-secret", BodyLength: 138, Snippet: "private-secret", CorrelationID: "private-secret"}
	code, message := authenticationError(err)
	if code != "auth_redirect_missing" || !strings.Contains(message, "REDIRECT_NO_LOCATION") || !strings.Contains(message, "URLSession") || strings.Contains(message, "private-") {
		t.Fatal(message)
	}
	err.Protocol = "private-secret"
	err.Method = "private-secret"
	err.AuthPath = "private-secret"
	err.ContentType = "private-secret"
	_, message = authenticationError(err)
	if strings.Contains(message, "private-") {
		t.Fatal(message)
	}
}
