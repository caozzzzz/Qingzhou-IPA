package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type catalogApp struct {
	ID        int64    `json:"id"`
	Name      string   `json:"name"`
	BundleID  string   `json:"bundleId"`
	Version   string   `json:"version"`
	UpdatedAt string   `json:"updatedAt"`
	SizeBytes int64    `json:"sizeBytes"`
	Developer string   `json:"developer"`
	MinimumOS string   `json:"minimumOS"`
	Price     *float64 `json:"price"`
	Currency  string   `json:"currency"`
	Platforms []string `json:"platforms"`
	Artwork   string   `json:"artwork"`
}

var appIDPattern = regexp.MustCompile(`^(?:id)?([1-9][0-9]{0,15})$`)
var catalogEndpoint = "https://itunes.apple.com"

func catalogSearch(query, country, platform string) ([]catalogApp, error) {
	query = strings.TrimSpace(query)
	if query == "" || len(query) > 400 {
		return nil, errors.New("invalid query")
	}
	if !regexp.MustCompile(`^[a-z]{2}$`).MatchString(country) {
		return nil, errors.New("invalid country")
	}
	entity := "software"
	if platform == "ipad" {
		entity = "iPadSoftware"
	} else if platform != "iphone" {
		return nil, errors.New("invalid platform")
	}
	params := url.Values{"country": {country}, "entity": {entity}, "media": {"software"}, "limit": {"30"}}
	endpoint := "/search"
	if match := appIDPattern.FindStringSubmatch(query); match != nil {
		endpoint = "/lookup"
		params.Set("id", match[1])
	} else {
		params.Set("term", query)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, catalogEndpoint+endpoint+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "QingzhouIPA/0.4")
	client := &http.Client{Timeout: 35 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, errors.New("catalog rejected query")
	}
	var result struct {
		Results []struct {
			ID        int64    `json:"trackId"`
			Name      string   `json:"trackName"`
			BundleID  string   `json:"bundleId"`
			Version   string   `json:"version"`
			Updated   string   `json:"currentVersionReleaseDate"`
			Released  string   `json:"releaseDate"`
			Size      string   `json:"fileSizeBytes"`
			Developer string   `json:"artistName"`
			MinimumOS string   `json:"minimumOsVersion"`
			Price     *float64 `json:"price"`
			Currency  string   `json:"currency"`
			Devices   []string `json:"supportedDevices"`
			Artwork   string   `json:"artworkUrl100"`
			Kind      string   `json:"kind"`
		} `json:"results"`
	}
	if err = json.NewDecoder(io.LimitReader(res.Body, 8<<20)).Decode(&result); err != nil {
		return nil, err
	}
	apps := []catalogApp{}
	seen := map[int64]bool{}
	for _, a := range result.Results {
		if a.ID <= 0 || a.Name == "" || seen[a.ID] || a.Kind == "mac-software" {
			continue
		}
		platforms := []string{}
		hasPhone, hasPad := false, false
		for _, d := range a.Devices {
			if strings.HasPrefix(d, "iPhone") || strings.HasPrefix(d, "iPod") {
				hasPhone = true
			}
			if strings.HasPrefix(d, "iPad") {
				hasPad = true
			}
		}
		if hasPhone {
			platforms = append(platforms, "iphone")
		}
		if hasPad {
			platforms = append(platforms, "ipad")
		}
		if len(a.Devices) > 0 && ((platform == "ipad" && !hasPad) || (platform == "iphone" && !hasPhone)) {
			continue
		}
		size, _ := strconv.ParseInt(a.Size, 10, 64)
		updated := a.Updated
		if updated == "" {
			updated = a.Released
		}
		apps = append(apps, catalogApp{a.ID, a.Name, a.BundleID, a.Version, updated, size, a.Developer, a.MinimumOS, a.Price, a.Currency, platforms, a.Artwork})
		seen[a.ID] = true
	}
	return apps, nil
}
