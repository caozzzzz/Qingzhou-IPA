package smsauth

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"howett.net/plist"
)

const resendDelay = 45 * time.Second

type Phone struct {
	ID    int    `json:"id"`
	Label string `json:"label"`
}

type State struct {
	Phones      []Phone `json:"phones"`
	SentPhoneID int     `json:"sentPhoneID"`
	RetryAfter  int     `json:"retryAfter"`
}

// Session lives only for the current login attempt. The App Store login
// still needs to issue its own session after GrandSlam verifies the code.
type Session struct {
	client        *Client
	identityToken string
	deviceID      string
	expires       time.Time
	phones        []Phone
	sentPhoneID   int
	nextSend      time.Time
	sendAttempts  int
	verifyCount   int
}

func (c *Client) Begin(ctx context.Context, email, password string, expires time.Time) (*Session, error) {
	ctx, cancel := context.WithDeadline(ctx, expires)
	defer cancel()
	if err := c.loadEndpoints(ctx); err != nil {
		return nil, err
	}
	device, err := c.proofHeaders(ctx)
	if err != nil {
		return nil, err
	}
	cpd := map[string]any{
		"bootstrap": "true", "icscrec": "true", "loc": "en_US",
		"pbe": "false", "prkgen": "true", "svct": "iCloud",
	}
	for _, name := range []string{"X-Mme-Device-Id", "X-Apple-I-MD", "X-Apple-I-MD-M"} {
		cpd[name] = device[name]
	}
	srp, err := newSRPClient()
	if err != nil {
		return nil, &Error{Stage: "SMS_SRP"}
	}
	var initial gsaReply
	version := map[string]any{"Version": "1.0.1"}
	err = c.applePlist(ctx, c.endpoints.Service, "SMS_SRP_INIT", version, map[string]any{
		"o": "init", "u": email, "A2k": srp.public.Bytes(), "ps": []string{"s2k", "s2k_fo"}, "cpd": cpd,
	}, nil, &initial)
	if err != nil {
		return nil, err
	}
	if initial.Status.Code != 0 || initial.Context == "" || len(initial.Context) > 8192 {
		return nil, &Error{Stage: "SMS_SRP_INIT", Code: initial.Status.Code}
	}
	proof, err := srp.proof(email, password, initial.Protocol, initial.Salt, initial.Public, initial.Iterations)
	if err != nil {
		return nil, err
	}
	defer clear(proof.key)
	defer clear(proof.premaster)
	var complete gsaReply
	err = c.applePlist(ctx, c.endpoints.Service, "SMS_SRP_COMPLETE", version, map[string]any{
		"o": "complete", "u": email, "c": initial.Context, "M1": proof.client, "cpd": cpd,
	}, nil, &complete)
	if err != nil {
		return nil, err
	}
	if complete.Status.Code != 0 {
		return nil, &Error{Stage: "SMS_SRP_COMPLETE", Code: complete.Status.Code}
	}
	action := complete.Status.Action
	if action == "" {
		action = complete.Action
	}
	switch action {
	case "secondaryAuth", "trustedDeviceSecondaryAuth", "sms", "phone", "trustedDevice", "pushedCode":
	case "":
		if complete.Status.HTTP != http.StatusConflict {
			return nil, &Error{Stage: "SMS_CHALLENGE"}
		}
	default:
		return nil, &Error{Stage: "SMS_CHALLENGE"}
	}
	var account struct {
		ID          string `plist:"adsid"`
		Token       string `plist:"GsIdmsToken"`
		LegacyToken string `plist:"idmsToken"`
	}
	if err = proof.decrypt(complete.Proof, complete.Extra, &account); err != nil {
		return nil, err
	}
	if account.Token == "" {
		account.Token = account.LegacyToken
	}
	if account.ID == "" || len(account.ID) > 24 || strings.IndexFunc(account.ID, func(r rune) bool { return r < '0' || r > '9' }) >= 0 ||
		account.Token == "" || len(account.Token) > 8192 {
		return nil, &Error{Stage: "SMS_CHALLENGE"}
	}
	session := &Session{
		client: c, expires: expires,
		deviceID:      device["X-Mme-Device-Id"],
		identityToken: base64.StdEncoding.EncodeToString([]byte(account.ID + ":" + account.Token)),
	}
	if err = session.loadPhones(ctx); err != nil {
		return nil, err
	}
	return session, nil
}

func (s *Session) Active() bool {
	return s != nil && s.identityToken != "" && time.Now().Before(s.expires)
}

func (s *Session) State() State {
	retry := int(math.Ceil(time.Until(s.nextSend).Seconds()))
	return State{Phones: s.phones, SentPhoneID: s.sentPhoneID, RetryAfter: max(0, retry)}
}

type phoneRecord struct {
	ID     json.RawMessage `json:"id"`
	Last   string          `json:"lastTwoDigits"`
	Number string          `json:"numberWithDialCode"`
	Masked string          `json:"obfuscatedNumber"`
}

type phoneReply struct {
	Trusted []phoneRecord     `json:"trustedPhoneNumbers"`
	Numbers []phoneRecord     `json:"phoneNumbers"`
	Phone   *phoneRecord      `json:"phoneNumber"`
	Errors  []json.RawMessage `json:"serviceErrors"`
	Other   []json.RawMessage `json:"errors"`
	Success *bool             `json:"success"`
	Code    int               `json:"ec"`
	Status  struct {
		Code int `json:"ec"`
	} `json:"Status"`
}

func (p phoneRecord) sanitized() (Phone, bool) {
	id, err := strconv.Atoi(strings.Trim(string(p.ID), "\""))
	if err != nil || id <= 0 || id > 1_000_000 {
		return Phone{}, false
	}
	for _, candidate := range []string{p.Last, p.Number, p.Masked} {
		tail := strings.TrimSpace(candidate)
		if len(tail) < 2 {
			continue
		}
		last := tail[len(tail)-2:]
		if last[0] >= '0' && last[0] <= '9' && last[1] >= '0' && last[1] <= '9' {
			return Phone{ID: id, Label: "尾号 " + last}, true
		}
	}
	return Phone{}, false
}

func parsePhoneReply(data []byte, status int, stage string) (phoneReply, error) {
	var reply phoneReply
	if status != http.StatusOK && status != http.StatusCreated && status != http.StatusNoContent {
		return reply, &Error{Stage: stage, HTTP: status}
	}
	data = bytes.TrimSpace(data)
	if len(data) != 0 && (data[0] == '<' || bytes.HasPrefix(data, []byte("bplist"))) {
		var values map[string]any
		if _, err := plist.Unmarshal(data, &values); err != nil || values == nil {
			return reply, &Error{Stage: stage, HTTP: status}
		}
		var err error
		data, err = json.Marshal(values)
		if err != nil {
			return reply, &Error{Stage: stage, HTTP: status}
		}
		defer clear(data)
	}
	if len(data) != 0 && (data[0] != '{' || json.Unmarshal(data, &reply) != nil) {
		return reply, &Error{Stage: stage, HTTP: status}
	}
	if len(reply.Errors) != 0 || len(reply.Other) != 0 || (reply.Success != nil && !*reply.Success) {
		return reply, &Error{Stage: stage, HTTP: status}
	}
	if reply.Code != 0 || reply.Status.Code != 0 {
		code := reply.Code
		if code == 0 {
			code = reply.Status.Code
		}
		return reply, &Error{Stage: stage, HTTP: status, Code: code}
	}
	return reply, nil
}

func (s *Session) phoneRequest(ctx context.Context, method, address, stage string, value any) (int, phoneReply, error) {
	if !s.Active() {
		return 0, phoneReply{}, &Error{Stage: "SMS_EXPIRED"}
	}
	ctx, cancel := context.WithDeadline(ctx, s.expires)
	defer cancel()
	headers, err := s.client.proofHeaders(ctx)
	if err != nil {
		return 0, phoneReply{}, err
	}
	if headers["X-Mme-Device-Id"] != s.deviceID {
		return 0, phoneReply{}, &Error{Stage: "SMS_EXPIRED"}
	}
	for k, v := range appleHeaders() {
		headers[k] = v
	}
	headers["X-Apple-Identity-Token"] = s.identityToken
	status, raw, err := jsonRequest(ctx, s.client.apple, method, address, appleHost, stage, value, headers)
	if err != nil {
		return status, phoneReply{}, err
	}
	defer clear(raw)
	reply, err := parsePhoneReply(raw, status, stage)
	return status, reply, err
}

func (s *Session) loadPhones(ctx context.Context) error {
	const stage = "SMS_PHONE_LIST"
	status, reply, err := s.phoneRequest(ctx, http.MethodGet, phoneListURL, stage, nil)
	if err != nil {
		return err
	}
	phones := append(reply.Trusted, reply.Numbers...)
	if reply.Phone != nil {
		phones = append([]phoneRecord{*reply.Phone}, phones...)
	}
	seen := map[int]bool{}
	for _, raw := range phones {
		phone, ok := raw.sanitized()
		if ok && !seen[phone.ID] {
			s.phones = append(s.phones, phone)
			seen[phone.ID] = true
		}
		if len(s.phones) == 10 {
			break
		}
	}
	if len(s.phones) == 0 {
		return &Error{Stage: stage, HTTP: status}
	}
	// HTTP 201 on this endpoint means Apple already initiated delivery.
	if status == http.StatusCreated {
		first, ok := phones[0].sanitized()
		if !ok {
			return &Error{Stage: stage, HTTP: status}
		}
		s.sentPhoneID = first.ID
		s.nextSend = time.Now().Add(resendDelay)
		s.sendAttempts = 1
	}
	return nil
}

func (s *Session) Send(ctx context.Context, phoneID int) error {
	if !s.Active() {
		return &Error{Stage: "SMS_EXPIRED"}
	}
	if s.sendAttempts >= 3 {
		return &Error{Stage: "SMS_SEND_LIMIT"}
	}
	if time.Now().Before(s.nextSend) {
		return &Error{Stage: "SMS_COOLDOWN"}
	}
	allowed := false
	for _, phone := range s.phones {
		if phone.ID == phoneID {
			allowed = true
			break
		}
	}
	if !allowed {
		return &Error{Stage: "SMS_PHONE_LIST"}
	}
	s.sendAttempts++
	s.nextSend = time.Now().Add(resendDelay)
	_, _, err := s.phoneRequest(ctx, http.MethodPut, smsURL, "SMS_SEND", map[string]any{
		"phoneNumber": map[string]int{"id": phoneID}, "mode": "sms",
	})
	if err == nil {
		s.sentPhoneID = phoneID
	}
	return err
}

func (s *Session) Verify(ctx context.Context, code string) error {
	if s.sentPhoneID <= 0 || len(code) != 6 || strings.IndexFunc(code, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
		return &Error{Stage: "SMS_CODE"}
	}
	if s.verifyCount >= 5 {
		return &Error{Stage: "SMS_VERIFY_LIMIT"}
	}
	s.verifyCount++
	_, _, err := s.phoneRequest(ctx, http.MethodPost, verifyURL, "SMS_VERIFY", map[string]any{
		"phoneNumber": map[string]int{"id": s.sentPhoneID},
		"mode":        "sms", "securityCode": map[string]string{"code": code},
	})
	if err == nil {
		s.identityToken = ""
	}
	return err
}
