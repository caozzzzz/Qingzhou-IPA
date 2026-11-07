package main

import (
	"errors"
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
