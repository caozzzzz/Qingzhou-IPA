package smsauth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"howett.net/plist"
)

const (
	appleHost    = "gsa.apple.com"
	proofHost    = "new.ani.sidestore.io"
	lookupURL    = "https://" + appleHost + "/grandslam/GsService2/lookup"
	phoneListURL = "https://" + appleHost + "/auth"
	smsURL       = "https://" + appleHost + "/auth/verify/phone"
	verifyURL    = smsURL + "/securitycode"
	clientInfo   = "<MacBookPro18,3> <macOS;26.6;25F84> <com.apple.AuthKit/1 (com.apple.akd/1.0)>"
	userAgent    = "AuthKit/1 (Macintosh; OS X 26.6) (com.apple.akd/1.0)"
)

// Error contains only fixed stage names and numeric Apple status codes.
type Error struct {
	Stage string
	HTTP  int
	Code  int
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s (HTTP %d, Apple code %d)", e.Stage, e.HTTP, e.Code)
}

type Client struct {
	apple, device *http.Client
	cacheDir      string
	identity      identity
	endpoints     endpoints
}

func New(cacheDir string) *Client {
	return &Client{
		apple:    isolatedClient(),
		device:   isolatedClient(),
		cacheDir: cacheDir,
	}
}

func isolatedClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DisableKeepAlives = true
	return &http.Client{
		Transport:     transport,
		Timeout:       20 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func exactHTTPS(raw, host string) (*url.URL, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != host || u.User != nil ||
		u.Fragment != "" || u.RawPath != "" || u.Opaque != "" || u.RawQuery != "" || u.ForceQuery {
		return nil, false
	}
	return u, true
}

func request(ctx context.Context, client *http.Client, method, address, host, contentType, stage string, body []byte, headers map[string]string) (int, []byte, error) {
	if _, ok := exactHTTPS(address, host); !ok {
		return 0, nil, &Error{Stage: stage}
	}
	req, err := http.NewRequestWithContext(ctx, method, address, bytes.NewReader(body))
	if err != nil {
		return 0, nil, &Error{Stage: stage}
	}
	req.Header.Set("Accept", contentType)
	if len(body) != 0 {
		req.Header.Set("Content-Type", contentType)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	res, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return 0, nil, ctx.Err()
		}
		return 0, nil, &Error{Stage: stage}
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return res.StatusCode, nil, &Error{Stage: stage, HTTP: res.StatusCode}
	}
	return res.StatusCode, data, nil
}

func appleHeaders() map[string]string {
	return map[string]string{
		"X-Mme-Client-Info": clientInfo,
		"User-Agent":        userAgent,
		"X-Xcode-Version":   "14.2 (14C18)",
		"X-Apple-App-Info":  "com.apple.gs.xcode.auth",
	}
}

type endpoints struct {
	Service string `plist:"gsService"`
	Start   string `plist:"midStartProvisioning"`
	Finish  string `plist:"midFinishProvisioning"`
}

func (c *Client) loadEndpoints(ctx context.Context) error {
	if c.endpoints.Service != "" {
		return nil
	}
	const stage = "SMS_CONFIG"
	status, data, err := request(ctx, c.apple, http.MethodGet, lookupURL, appleHost, "text/x-xml-plist", stage, nil, appleHeaders())
	if err != nil {
		return err
	}
	var envelope struct {
		URLs endpoints `plist:"urls"`
	}
	if status != http.StatusOK {
		return &Error{Stage: stage, HTTP: status}
	}
	if _, err = plist.Unmarshal(data, &envelope); err != nil {
		return &Error{Stage: stage}
	}
	for _, address := range []string{envelope.URLs.Service, envelope.URLs.Start, envelope.URLs.Finish} {
		u, ok := exactHTTPS(address, appleHost)
		if !ok || !strings.HasPrefix(u.Path, "/grandslam/") {
			return &Error{Stage: stage}
		}
	}
	c.endpoints = envelope.URLs
	return nil
}

type gsaStatus struct {
	Code   int    `plist:"ec"`
	HTTP   int    `plist:"hsc"`
	Action string `plist:"au"`
}

type gsaReply struct {
	Status     gsaStatus `plist:"Status"`
	Action     string    `plist:"au"`
	Salt       []byte    `plist:"s"`
	Public     []byte    `plist:"B"`
	Iterations int       `plist:"i"`
	Protocol   string    `plist:"sp"`
	Context    string    `plist:"c"`
	Proof      []byte    `plist:"M2"`
	Extra      []byte    `plist:"spd"`
}

func (c *Client) applePlist(ctx context.Context, address, stage string, header, params map[string]any, extra map[string]string, output any) error {
	data, err := plist.Marshal(map[string]any{"Header": header, "Request": params}, plist.XMLFormat)
	if err != nil {
		return &Error{Stage: stage}
	}
	headers := appleHeaders()
	for k, v := range extra {
		headers[k] = v
	}
	status, raw, err := request(ctx, c.apple, http.MethodPost, address, appleHost, "text/x-xml-plist", stage, data, headers)
	clear(data)
	if err != nil {
		return err
	}
	defer clear(raw)
	if status != http.StatusOK {
		return &Error{Stage: stage, HTTP: status}
	}
	var envelope map[string]any
	if _, err = plist.Unmarshal(raw, &envelope); err != nil {
		return &Error{Stage: stage}
	}
	response, ok := envelope["Response"].(map[string]any)
	if !ok {
		return &Error{Stage: stage}
	}
	inner, err := plist.Marshal(response, plist.BinaryFormat)
	if err != nil {
		return &Error{Stage: stage}
	}
	defer clear(inner)
	if _, err = plist.Unmarshal(inner, output); err != nil {
		return &Error{Stage: stage}
	}
	return nil
}

func jsonRequest(ctx context.Context, client *http.Client, method, address, host, stage string, value any, headers map[string]string) (int, []byte, error) {
	var payload []byte
	if value != nil {
		var err error
		payload, err = json.Marshal(value)
		if err != nil {
			return 0, nil, &Error{Stage: stage}
		}
	}
	defer clear(payload)
	return request(ctx, client, method, address, host, "application/json", stage, payload, headers)
}

func errorIsStage(err error, stage string) bool {
	var failure *Error
	return errors.As(err, &failure) && failure.Stage == stage
}
