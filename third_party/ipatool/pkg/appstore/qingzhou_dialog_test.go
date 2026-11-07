package appstore

import (
	"errors"
	"github.com/majd/ipatool/v2/pkg/http"
	"testing"
)

func TestAccountDialogCannotBecomeSuccessfulLogin(t *testing.T) {
	sut := &appstore{}
	response := http.Result[loginResult]{StatusCode: 200, Data: loginResult{Dialog: map[string]interface{}{"message": "private-message"}}}
	retry, _, err := sut.parseLoginResponse(&response, 1, "", "https://buy.itunes.apple.com/WebObjects/MZFinance.woa/wa/authenticate/")
	if retry || !errors.Is(err, ErrAuthInteractionRequired) {
		t.Fatal("account dialog was not preserved as an explicit failure")
	}
}
