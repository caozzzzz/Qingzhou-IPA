package appstore

import (
	"context"
	"errors"
	"fmt"
	gohttp "net/http"
	"time"

	"github.com/majd/ipatool/v2/internal/store"
	apphttp "github.com/majd/ipatool/v2/pkg/http"
)

// QingzhouCheckAuthentication exercises the bag, resource loader and SAP signing
// session and probes the configured entry with a credential-free GET.
// A response status shows reachability. A probe error is separate from local setup.
func QingzhouCheckAuthentication(ctx context.Context, client AppStore, mode string) (status int, probeErr, setupErr error) {
	t, ok := client.(*appstore)
	if !ok {
		return 0, nil, errors.New("unsupported authentication client")
	}
	mac, err := t.machine.MacAddress()
	if err != nil {
		return 0, nil, err
	}
	guid, machineID, err := machineIdentity(mac)
	if err != nil {
		return 0, nil, err
	}
	bag, err := t.bag(guid)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to get bag: %w", err)
	}
	signer, err := t.actionSignerFactory(bag.SAPConfig, machineID)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to initialize SAP action signer: %w", err)
	}
	defer signer.Close()
	signature, err := signer.Sign([]byte("Qingzhou authentication environment check"))
	if err != nil {
		return 0, nil, err
	}
	if len(signature) == 0 {
		return 0, nil, errors.New("SAP signing returned no signature")
	}
	if err := store.CheckMachineProof(ctx, guid); err != nil {
		return 0, nil, err
	}
	status, probeErr = probeAuthenticationEndpoint(ctx, bag.AuthEndpoint, mode)
	return status, probeErr, nil
}

func probeAuthenticationEndpoint(ctx context.Context, endpoint, mode string) (int, error) {
	if err := validateAuthenticationEndpoint(endpoint); err != nil {
		return 0, err
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := gohttp.NewRequestWithContext(ctx, gohttp.MethodGet, endpoint, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", apphttp.DefaultUserAgent)
	transport := apphttp.AuthenticationRoundTripper(nil)
	if mode == apphttp.AuthenticationTransportGo || transport == nil {
		isolated := gohttp.DefaultTransport.(*gohttp.Transport).Clone()
		isolated.DisableKeepAlives = true
		defer isolated.CloseIdleConnections()
		transport = isolated
	}
	client := gohttp.Client{Transport: transport, CheckRedirect: func(*gohttp.Request, []*gohttp.Request) error {
		return gohttp.ErrUseLastResponse
	}}
	response, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	return response.StatusCode, nil
}
