package appstore

import (
	"errors"
	"strings"
)

// AuthenticationFailure carries only structural diagnostics. Response bodies,
// account records, tokens and Apple customer messages stay outside these fields.
type AuthenticationFailure struct {
	StatusCode   int
	FailureType  string
	Verification bool
	Disabled     bool
	cause        error
}

func (e *AuthenticationFailure) Error() string {
	if e.cause == nil {
		return "Apple authentication failed"
	}
	return e.cause.Error()
}
func (e *AuthenticationFailure) Unwrap() error { return e.cause }

func loginFailure(err error, status int, failureType, customerMessage string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrAuthCodeRequired) {
		return ErrAuthCodeRequired
	}
	safe := len(failureType) > 0 && len(failureType) <= 24
	for _, r := range failureType {
		if !(r >= '0' && r <= '9' || r == '-') {
			safe = false
		}
	}
	if !safe {
		failureType = "unknown"
	}
	message := strings.ToLower(customerMessage)
	return &AuthenticationFailure{StatusCode: status, FailureType: failureType,
		Verification: strings.Contains(message, "verification") || strings.Contains(message, "two-factor") || strings.Contains(message, "验证码") || strings.Contains(message, "双重认证"),
		Disabled:     customerMessage == CustomerMessageAccountDisabled, cause: err}
}
