package smsauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

type fixtureTransport func(*http.Request) (*http.Response, error)

func (f fixtureTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func fixtureResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
}

func TestSMSDeliveryAndVerificationUseTheSameTrustedPhone(t *testing.T) {
	client := New("", nil)
	client.identity = identity{Identifier: make([]byte, 16), Provision: []byte("fixture-device")}
	client.device.Transport = fixtureTransport(func(req *http.Request) (*http.Response, error) {
		var body map[string]json.RawMessage
		if req.URL.Host != proofHost || req.URL.Path != "/v3/get_headers" || req.Header.Get("X-Apple-Identity-Token") != "" ||
			json.NewDecoder(req.Body).Decode(&body) != nil || len(body) != 2 || body["identifier"] == nil || body["adi_pb"] == nil {
			t.Fatal("unexpected material was sent to the proof service")
		}
		return fixtureResponse(200, `{"result":"Headers","X-Apple-I-MD":"YWJj","X-Apple-I-MD-M":"eHl6","X-Apple-I-MD-RINFO":"0"}`), nil
	})
	sendCalls, verifyCalls := 0, 0
	client.apple.Transport = fixtureTransport(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != appleHost || req.Header.Get("X-Apple-Identity-Token") != "fixture-token" {
			t.Fatal("wrong Apple session")
		}
		switch req.URL.Path {
		case "/auth":
			return fixtureResponse(200, `{"trustedPhoneNumbers":[{"id":7,"numberWithDialCode":"+1 555 000 1283"},{"id":"9","lastTwoDigits":"51"},{"id":10,"lastTwoDigits":"19","pushMode":"voice"}]}`), nil
		case "/auth/verify/phone", "/auth/verify/phone/securitycode":
			var body struct {
				Phone struct{ ID int } `json:"phoneNumber"`
				Mode  string
				Code  struct{ Code string } `json:"securityCode"`
			}
			if json.NewDecoder(req.Body).Decode(&body) != nil || body.Phone.ID != 9 || body.Mode != "sms" {
				t.Fatal("wrong number or delivery method")
			}
			if req.URL.Path == "/auth/verify/phone" {
				if req.Method != http.MethodPut || body.Code.Code != "" {
					t.Fatal("invalid SMS send request")
				}
				sendCalls++
				if sendCalls == 1 {
					return fixtureResponse(200, `{"Status":{"ec":-21669}}`), nil
				}
				return fixtureResponse(204, ""), nil
			}
			verifyCalls++
			if req.Method != http.MethodPost || body.Code.Code != "012345" {
				t.Fatal("invalid SMS verification request")
			}
			if verifyCalls == 1 {
				return fixtureResponse(403, `{"serviceErrors":[{}]}`), nil
			}
			return fixtureResponse(200, "{}"), nil
		default:
			t.Fatal("unexpected request path")
			return nil, fmt.Errorf("unexpected request")
		}
	})
	session := &Session{
		client: client, identityToken: "fixture-token", expires: time.Now().Add(time.Minute),
		deviceID: client.identity.deviceHeaders()["X-Mme-Device-Id"],
	}
	ctx := context.Background()
	if err := session.loadPhones(ctx); err != nil || len(session.phones) != 2 || session.phones[0].Label != "尾号 83" {
		t.Fatal("trusted numbers did not load with masked labels", err)
	}
	if err := session.Send(ctx, 999); !errorIsStage(err, "SMS_PHONE_LIST") || sendCalls != 0 {
		t.Fatal("untrusted number was accepted")
	}
	if err := session.Send(ctx, 9); !errorIsStage(err, "SMS_SEND") || session.State().SentPhoneID != 0 {
		t.Fatal("Apple rejection was treated as a send")
	}
	if err := session.Send(ctx, 9); !errorIsStage(err, "SMS_COOLDOWN") || sendCalls != 1 {
		t.Fatal("send cooldown was bypassed")
	}
	session.nextSend = time.Time{}
	if err := session.Send(ctx, 9); err != nil || session.State().SentPhoneID != 9 {
		t.Fatal("accepted send was not recorded", err)
	}
	if err := session.Verify(ctx, "012345"); !errorIsStage(err, "SMS_VERIFY") || !session.Active() {
		t.Fatal("wrong code closed the current challenge")
	}
	if err := session.Verify(ctx, "012345"); err != nil || session.Active() {
		t.Fatal("successful verification did not consume the challenge", err)
	}
}

func TestAppleSMSDeviceSetupLive(t *testing.T) {
	if os.Getenv("QZ_SMS_LIVE_PROBE") != "1" {
		t.Skip("optional unauthenticated service probe")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 85*time.Second)
	defer cancel()
	client := New(t.TempDir(), nil)
	if err := client.loadEndpoints(ctx); err != nil {
		t.Fatal(err)
	}
	t.Log("Apple SMS configuration returned the expected endpoints")
	if _, err := client.proofHeaders(ctx); err != nil {
		t.Fatal(err)
	}
	t.Log("SideStore and Apple provisioned an anonymous device proof")
}
