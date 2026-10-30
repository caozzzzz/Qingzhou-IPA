package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/majd/ipatool/v2/pkg/appstore"
)

func TestNeverStoresPassword(t *testing.T) {
	k:=&memoryKeychain{values:map[string][]byte{}}
	data,_:=json.Marshal(appstore.Account{Email:"test@example.invalid",Password:"do-not-store-this",PasswordToken:"session-token"})
	if err:=k.Set("account",data);err!=nil{t.Fatal(err)}
	saved,_:=k.Get("account")
	if strings.Contains(string(saved),"do-not-store-this") || strings.Contains(string(saved),`"password":`){t.Fatal("password persisted")}
	if !strings.Contains(string(saved),"session-token"){t.Fatal("session lost")}
}
func TestCookieDomainIsolationAndExpiry(t *testing.T) {
	j:=newJar();apple,_:=url.Parse("https://buy.itunes.apple.com/WebObjects/login");other,_:=url.Parse("https://example.invalid/")
	j.SetCookies(apple,[]*http.Cookie{{Name:"session",Value:"secret",Path:"/",Secure:true,MaxAge:60},{Name:"old",Value:"expired",Path:"/",Expires:time.Now().Add(-time.Hour)}})
	if len(j.Cookies(other))!=0{t.Fatal("cookie crossed domain")}
	if len(j.Cookies(apple))!=1{t.Fatal("expiry not applied")}
	snapshot:=j.snapshot();if len(snapshot)!=1{t.Fatal("expired cookie exported")}
	if snapshot[0].Cookie.MaxAge!=0 || snapshot[0].Cookie.Expires.IsZero(){t.Fatal("relative lifetime could be extended on restore")}
	j2:=newJar();for _,r:=range snapshot{u,_:=url.Parse(r.URL);j2.SetCookies(u,[]*http.Cookie{r.Cookie})}
	if len(j2.Cookies(apple))!=1{t.Fatal("session restore failed")}
}
func TestProtocolValidationWithoutNetwork(t *testing.T) {
	b:=&bridge{}
	if !b.handle(request{ID:7,Action:"ping"}).OK{t.Fatal("ping failed")}
	r:=b.handle(request{Action:"login",GUID:"020102030405",Email:"",Password:""});if r.OK || r.ErrorCode!="input"{t.Fatal("empty credentials accepted")}
	r=b.handle(request{Action:"login",GUID:"020102030405",Email:"x",Password:"p",Code:"abc123"});if r.OK || r.ErrorCode!="input"{t.Fatal("invalid 2FA accepted")}
	r=b.handle(request{Action:"restore",GUID:"020102030405",Session:&savedSession{Account:appstore.Account{Email:"x"}}});if r.OK{t.Fatal("invalid session accepted")}
	if !b.handle(request{Action:"logout",GUID:"020102030405"}).OK{t.Fatal("logout failed")}
}
