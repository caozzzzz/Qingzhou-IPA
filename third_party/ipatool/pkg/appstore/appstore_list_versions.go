package appstore

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/majd/ipatool/v2/pkg/http"
)

type ListVersionsInput struct {
	Context  context.Context
	Account  Account
	App      App
	Platform Platform
}

type ListVersionsOutput struct {
	ExternalVersionIdentifiers []string
	LatestExternalVersionID    string
}

func (t *appstore) ListVersions(input ListVersionsInput) (ListVersionsOutput, error) {
	platform := input.Platform
	if platform == "" {
		platform = PlatformIPhone
	}

	switch platform {
	case PlatformIPhone, PlatformIPad, PlatformAppleTV, PlatformWatchOS, PlatformVisionOS, PlatformMacOS:
	default:
		return ListVersionsOutput{}, fmt.Errorf("invalid platform %q", platform)
	}

	macAddr, err := t.machine.MacAddress()
	if err != nil {
		return ListVersionsOutput{}, fmt.Errorf("failed to get mac address: %w", err)
	}

	guid := strings.ReplaceAll(strings.ToUpper(macAddr), ":", "")

	var externalVersionID string

	switch platform {
	case PlatformMacOS:
		externalVersionID, err = t.lookupLatestMacOSExternalVersionID(input.Account, input.App)
	case PlatformAppleTV, PlatformWatchOS, PlatformVisionOS:
		externalVersionID, err = t.lookupLatestExternalVersionID(input.Account, input.App, platform)
	}

	if err != nil {
		return ListVersionsOutput{}, fmt.Errorf("failed to resolve platform version: %w", err)
	}

	res, _, err := t.sendDownloadProduct(input.Context, input.Account, input.App, guid, externalVersionID, platform)
	if err != nil {
		return ListVersionsOutput{}, err
	}

	return parseVersionListResponse(res)
}

func parseVersionListResponse(res http.Result[downloadResult]) (ListVersionsOutput, error) {
	if res.Data.FailureType == FailureTypePasswordTokenExpired || res.Data.FailureType == FailureTypeSignInRequired {
		return ListVersionsOutput{}, ErrPasswordTokenExpired
	}

	if res.Data.FailureType == FailureTypeLicenseNotFound {
		return ListVersionsOutput{}, ErrLicenseRequired
	}

	if res.Data.CustomerMessage != "" && (res.Data.FailureType != "" || len(res.Data.Items) == 0) {
		return ListVersionsOutput{}, NewErrorWithMetadata(fmt.Errorf("received error: %s", res.Data.CustomerMessage), res)
	}

	if res.Data.FailureType != "" {
		return ListVersionsOutput{}, NewErrorWithMetadata(fmt.Errorf("received error: %s", res.Data.FailureType), res)
	}

	if len(res.Data.Items) == 0 {
		return ListVersionsOutput{}, NewErrorWithMetadata(errors.New("invalid response"), res)
	}

	item := res.Data.Items[0]

	rawIdentifiers, ok := item.Metadata["softwareVersionExternalIdentifiers"].([]interface{})
	if !ok {
		return ListVersionsOutput{}, NewErrorWithMetadata(fmt.Errorf("failed to get version identifiers from item metadata"), item.Metadata)
	}

	externalVersionIdentifiers := make([]string, 0, len(rawIdentifiers))
	seen := map[string]bool{}
	for _, val := range rawIdentifiers {
		id, err := normalizeVersionIdentifier(val)
		if err != nil {
			return ListVersionsOutput{}, err
		}
		if !seen[id] {
			externalVersionIdentifiers = append(externalVersionIdentifiers, id)
			seen[id] = true
		}
	}

	latestExternalVersionID := item.Metadata["softwareVersionExternalIdentifier"]
	if latestExternalVersionID == nil {
		return ListVersionsOutput{}, NewErrorWithMetadata(fmt.Errorf("failed to get latest version from item metadata"), item.Metadata)
	}

	latestID, err := normalizeVersionIdentifier(latestExternalVersionID)
	if err != nil {
		return ListVersionsOutput{}, err
	}
	return ListVersionsOutput{
		ExternalVersionIdentifiers: externalVersionIdentifiers,
		LatestExternalVersionID:    latestID,
	}, nil
}

func normalizeVersionIdentifier(value interface{}) (string, error) {
	var text string
	switch v := value.(type) {
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 1 || v > 9007199254740991 || math.Trunc(v) != v {
			return "", errors.New("invalid numeric version identifier")
		}
		text = strconv.FormatFloat(v, 'f', 0, 64)
	case float32:
		return normalizeVersionIdentifier(float64(v))
	default:
		text = fmt.Sprint(value)
	}
	if !regexp.MustCompile(`^[0-9]{1,24}$`).MatchString(text) {
		return "", errors.New("invalid version identifier in store response")
	}
	return text, nil
}
