package appstore

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/majd/ipatool/v2/pkg/http"
)

// QingzhouListVersions selects only existing Apple endpoints. No session is sent
// to a third-party service and no caller-provided endpoint is accepted.
func QingzhouListVersions(client AppStore, input ListVersionsInput, source string) (ListVersionsOutput, error) {
	if source == "" || source == "auto" {
		return client.ListVersions(input)
	}
	t, ok := client.(*appstore)
	if !ok {
		return ListVersionsOutput{}, errors.New("unsupported store client")
	}
	if source != "download" && source != "compat" && source != "redownload" {
		return ListVersionsOutput{}, errors.New("unknown version source")
	}
	platform := input.Platform
	if platform == "" {
		platform = PlatformIPhone
	}
	if platform != PlatformIPhone && platform != PlatformIPad {
		return ListVersionsOutput{}, errors.New("source supports only iPhone/iPad")
	}
	ctx := input.Context
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return ListVersionsOutput{}, err
	}
	mac, err := t.machine.MacAddress()
	if err != nil {
		return ListVersionsOutput{}, err
	}
	guid := strings.ReplaceAll(strings.ToUpper(mac), ":", "")
	var res http.Result[downloadResult]
	if source == "compat" {
		req := t.downloadProductRequest(t.volumeStoreEndpoint(input.Account), input.Account, input.App, guid, "")
		req.Context = ctx
		res, err = t.downloadClient.Send(req)
	} else {
		bag, e := t.fetchURLBag(guid)
		if e != nil {
			return ListVersionsOutput{}, e
		}
		if source == "download" {
			if bag.EntDownloadEndpoint == "" {
				return ListVersionsOutput{}, errors.New("Apple download endpoint unavailable")
			}
			res, _, err = t.sendPreferredDownload(ctx, bag.EntDownloadEndpoint, input.Account, input.App, guid, "", platform)
		} else {
			if bag.RedownloadEndpoint == "" {
				return ListVersionsOutput{}, errors.New("Apple redownload endpoint unavailable")
			}
			endpoint, e := newDownloadEndpoint(bag.RedownloadEndpoint, redownloadProductPath)
			if e != nil {
				return ListVersionsOutput{}, e
			}
			latest, e := t.lookupLatestExternalVersionID(input.Account, input.App, platform)
			if e != nil {
				return ListVersionsOutput{}, e
			}
			req := t.downloadProductRequest(endpoint, input.Account, input.App, guid, latest)
			req.Context = ctx
			res, err = t.downloadClient.Send(req)
		}
	}
	if err != nil {
		return ListVersionsOutput{}, fmt.Errorf("version source failed: %w", err)
	}
	return parseVersionListResponse(res)
}
