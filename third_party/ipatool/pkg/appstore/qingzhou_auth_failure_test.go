package appstore

import (
	"errors"
	"testing"
)

func TestQingzhouLoginFailureKeepsChallengeIdentityAndSanitizesCode(t *testing.T) {
	err := loginFailure(ErrAuthCodeRequired, 200, "private-secret-token", "")
	if err != ErrAuthCodeRequired {
		t.Fatal("2FA challenge identity lost")
	}
	var failure *AuthenticationFailure
	err = loginFailure(errors.New("server details"), 200, "private-secret-token", "")
	if !errors.As(err, &failure) || failure.FailureType != "unknown" {
		t.Fatal("unexpected diagnostic")
	}
	err = loginFailure(errors.New("server details"), 200, "-5000", "验证码错误")
	if !errors.As(err, &failure) || failure.FailureType != "-5000" || !failure.Verification {
		t.Fatal("verification classification missing")
	}
}
