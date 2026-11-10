package store

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"strings"

	apphttp "github.com/majd/ipatool/v2/pkg/http"
)

type machineProofProvider func(context.Context, string, string, string) (map[string]string, error)

// CheckMachineProof initializes the local component without account credentials.
// It does not provision a device or verify an Apple account.
func CheckMachineProof(ctx context.Context, guid string) error {
	_, err := platformMachineProof(ctx, "check", "", guid)
	return err
}

func sendWithMachineProof(request *http.Request, guid string, send func(*http.Request) (*http.Response, error), provider machineProofProvider) (*http.Response, error) {
	if !needsStoreProof(request) {
		return send(request)
	}
	// Only the two proof headers may cross from the local device module.
	headers, err := provider(request.Context(), "headers", "", guid)
	if err != nil {
		if request.Context().Err() != nil {
			return nil, request.Context().Err()
		}
		// A corrupt or expired local identity must not permanently lock out
		// sign-in. Clear only device proof state and let Apple challenge again.
		if _, err = provider(request.Context(), "reset", "", guid); err != nil {
			return nil, err
		}
		headers = nil
	}
	reset, provisioned := false, false
	current := request
	for attempt := 0; attempt < 3; attempt++ {
		current = current.Clone(request.Context())
		current.Header.Del("X-Apple-AMD")
		current.Header.Del("X-Apple-AMD-M")
		for _, key := range []string{"X-Apple-AMD", "X-Apple-AMD-M"} {
			if v := headers[key]; v != "" {
				if len(v) > 16384 || strings.ContainsAny(v, "\r\n") {
					return nil, errors.New("invalid Store device proof")
				}
				current.Header.Set(key, v)
			}
		}
		if attempt > 0 && request.Body != nil {
			if request.GetBody == nil {
				return nil, errors.New("Store verification request cannot be replayed")
			}
			current.Body, err = request.GetBody()
			if err != nil {
				return nil, err
			}
		}
		res, err := send(current)
		if err != nil {
			return res, err
		}
		if res.StatusCode != http.StatusOK {
			return res, nil
		}
		action := strings.ToUpper(strings.TrimSpace(res.Header.Get("X-Apple-AMD-Action")))
		if action == "" {
			data, e := io.ReadAll(io.LimitReader(res.Body, (2<<20)+1))
			_ = res.Body.Close()
			res.Body = io.NopCloser(bytes.NewReader(data))
			if e != nil || len(data) > 2<<20 {
				return nil, errors.New("Store authentication response unreadable")
			}
			var raw map[string]any
			if e = apphttp.DecodePropertyList(data, &raw); e == nil {
				if msg, ok := raw["customerMessage"].(string); ok && strings.HasPrefix(msg, "AMD-Action:") {
					parts := strings.Split(msg, ":")
					action = strings.ToUpper(parts[len(parts)-1])
				}
			}
		}
		if action != "SP" && action != "RP" {
			return res, nil
		}
		_ = res.Body.Close()
		switch action {
		case "RP":
			if reset {
				return nil, errors.New("Store device reset repeated")
			}
			reset = true
			_, err = provider(request.Context(), "reset", "", guid)
			headers = nil
		case "SP":
			if provisioned {
				return nil, errors.New("Store device provisioning repeated")
			}
			provisioned = true
			challenge := res.Header.Get("X-Apple-AMD-Data")
			decoded, e := base64.StdEncoding.Strict().DecodeString(challenge)
			if e != nil || len(decoded) == 0 || len(challenge) > 16384 || base64.StdEncoding.EncodeToString(decoded) != challenge {
				return nil, errors.New("invalid Store device challenge")
			}
			headers, err = provider(request.Context(), "provision", challenge, guid)
			if err == nil && (headers["X-Apple-AMD"] == "" || headers["X-Apple-AMD-M"] == "") {
				return nil, errors.New("incomplete Store device verification")
			}
		}
		if err != nil {
			return nil, err
		}
	}
	return nil, errors.New("Store device verification retry limit reached")
}
