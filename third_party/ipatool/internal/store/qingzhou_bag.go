package store

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	apphttp "github.com/majd/ipatool/v2/pkg/http"
)

const authenticationBagLimit = 2 << 20

type bagTransport struct {
	name      string
	transport http.RoundTripper
}

func (b *Backend) fetchAuthenticationBag(ctx context.Context, address string) (*http.Response, error) {
	goChannel := bagTransport{apphttp.AuthenticationTransportGo, b.authTransport}
	nativeChannel := bagTransport{apphttp.AuthenticationTransportNative, apphttp.AuthenticationRoundTripper(nil)}
	channels := []bagTransport{goChannel}
	if nativeChannel.transport != nil {
		channels = []bagTransport{nativeChannel, goChannel}
		if apphttp.AuthenticationTransport() == apphttp.AuthenticationTransportGo {
			channels = []bagTransport{goChannel, nativeChannel}
		}
	}
	response, values, err := readAuthenticationBag(ctx, address, channels)
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	b.bag = values
	b.mu.Unlock()
	return response, nil
}

func readAuthenticationBag(ctx context.Context, address string, channels []bagTransport) (*http.Response, map[string]any, error) {
	endpoint, err := url.Parse(address)
	if err != nil || endpoint.Scheme != "https" || endpoint.Host != "init.itunes.apple.com" ||
		endpoint.Path != "/bag.xml" || endpoint.RawPath != "" || endpoint.User != nil || endpoint.Fragment != "" {
		return nil, nil, errors.New("invalid Apple authentication configuration URL")
	}
	query, err := url.ParseQuery(endpoint.RawQuery)
	if err != nil || len(query) != 1 || len(query["guid"]) != 1 {
		return nil, nil, errors.New("invalid Apple authentication configuration query")
	}
	guid := query.Get("guid")
	if decoded, err := hex.DecodeString(guid); err != nil || len(decoded) != 6 {
		return nil, nil, errors.New("invalid Apple authentication configuration identifier")
	}
	endpoint.RawQuery = "ix=6&guid=" + strings.ToUpper(guid)

	failure := &apphttp.AuthenticationBagError{}
	for _, channel := range channels {
		response, values, attempt := requestAuthenticationBag(ctx, endpoint.String(), channel)
		if attempt.Stage == "" {
			return response, values, nil
		}
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		failure.Attempts = append(failure.Attempts, attempt)
		if attempt.StatusCode == http.StatusTooManyRequests {
			break
		}
	}
	return nil, nil, failure
}

func requestAuthenticationBag(ctx context.Context, address string, channel bagTransport) (*http.Response, map[string]any, apphttp.AuthenticationBagAttempt) {
	client := http.Client{Transport: channel.transport, Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	attempt := apphttp.AuthenticationBagAttempt{Transport: channel.name}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		attempt.Stage, attempt.Cause = "SEND", err
		return nil, nil, attempt
	}
	request.Header.Set("Accept", "application/xml")
	request.Header.Set("User-Agent", apphttp.DefaultUserAgent)
	response, err := client.Do(request)
	if err != nil {
		attempt.Stage, attempt.Cause = "SEND", err
		return nil, nil, attempt
	}
	defer response.Body.Close()
	attempt.StatusCode = response.StatusCode
	if response.StatusCode != http.StatusOK {
		attempt.Stage = "HTTP"
		return nil, nil, attempt
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, authenticationBagLimit+1))
	if err != nil || len(data) > authenticationBagLimit {
		attempt.Stage, attempt.Cause = "READ", err
		return nil, nil, attempt
	}
	var values map[string]any
	if err := apphttp.DecodePropertyList(data, &values); err != nil || len(values) == 0 {
		attempt.Stage = "FORMAT"
		return nil, nil, attempt
	}
	response.Body = io.NopCloser(bytes.NewReader(data))
	return response, values, apphttp.AuthenticationBagAttempt{}
}
