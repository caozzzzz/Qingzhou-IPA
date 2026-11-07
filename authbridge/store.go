package main

import (
	"context"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/majd/ipatool/v2/pkg/appstore"
	"github.com/schollz/progressbar/v3"
)

var versionPattern = regexp.MustCompile(`^[0-9]{1,24}$`)

func storeError(err error) (string, string) {
	switch {
	case errors.Is(err, appstore.ErrPasswordTokenExpired):
		return "expired", "Apple 登录会话已过期，请退出后重新登录。"
	case errors.Is(err, appstore.ErrLicenseRequired):
		return "license", "此账户尚无该应用许可。免费应用可点击“获取免费许可”后重试；付费应用请先在 App Store 购买。"
	case errors.Is(err, appstore.ErrAppNotFound):
		return "notfound", "当前账户商店中找不到该应用。"
	default:
		return "store", "App Store 未能完成请求。请检查账户商店地区、应用许可、网络和所选版本是否仍可下载，再重试。"
	}
}
func (b *bridge) storeOperation(in request) response {
	out := response{ID: in.ID}
	platform, err := appstore.ParsePlatform(in.Platform)
	if err != nil || (platform != appstore.PlatformIPhone && platform != appstore.PlatformIPad) || in.AppID <= 0 {
		out.Error = "请选择 iPhone 或 iPad，并提供有效的 App ID。"
		out.ErrorCode = "input"
		return out
	}
	if in.VersionID != "" && !versionPattern.MatchString(in.VersionID) {
		out.Error = "版本 ID 无效。"
		out.ErrorCode = "input"
		return out
	}
	info, err := b.store.AccountInfo()
	if err != nil {
		out.Error = "请先登录 Apple 账户。"
		out.ErrorCode = "login"
		return out
	}
	acc := info.Account
	app := appstore.App{ID: in.AppID}
	// License acquisition is explicit, and catalog lookup is mandatory before acquiring a free license.
	if in.AcquireLicense {
		lookup, e := b.store.Lookup(appstore.LookupInput{Account: acc, AppID: in.AppID, Platform: platform})
		if e != nil {
			out.ErrorCode, out.Error = storeError(e)
			return out
		}
		app = lookup.App
		if app.Price > 0 {
			out.Error = "此工具不购买付费应用，请先在 App Store 完成购买。"
			out.ErrorCode = "paid"
			return out
		}
		if e = b.store.Purchase(appstore.PurchaseInput{Account: acc, App: app, Platform: platform}); e != nil && !errors.Is(e, appstore.ErrLicenseAlreadyExists) {
			out.ErrorCode, out.Error = storeError(e)
			return out
		}
	}
	switch in.Action {
	case "versions":
		source := in.Source
		if source == "" {
			source = "auto"
		}
		if source != "auto" && source != "combined" && source != "download" && source != "compat" && source != "redownload" {
			out.Error = "查询通道无效。"
			out.ErrorCode = "input"
			return out
		}
		sources := []string{source}
		if source == "combined" {
			sources = []string{"download", "compat", "redownload"}
		}
		ids := []string{}
		seen := map[string]bool{}
		latest := ""
		diagnostics := []map[string]interface{}{}
		var lastErr error
		success := false
		for _, channel := range sources {
			ctx, cancel := context.WithTimeout(in.operationContext(), 55*time.Second)
			result, e := appstore.QingzhouListVersions(b.store, appstore.ListVersionsInput{Context: ctx, Account: acc, App: app, Platform: platform}, channel)
			cancel()
			row := map[string]interface{}{"source": channel, "count": 0, "ok": e == nil}
			if e != nil {
				lastErr = e
				_, message := storeError(e)
				row["error"] = message
			} else {
				success = true
				if latest == "" {
					latest = result.LatestExternalVersionID
				}
				sourceIDs := append([]string{result.LatestExternalVersionID}, result.ExternalVersionIdentifiers...)
				local := map[string]bool{}
				for _, id := range sourceIDs {
					if versionPattern.MatchString(id) {
						local[id] = true
						if !seen[id] {
							seen[id] = true
							ids = append(ids, id)
						}
					}
				}
				row["count"] = len(local)
			}
			diagnostics = append(diagnostics, row)
		}
		if !success {
			out.ErrorCode, out.Error = storeError(lastErr)
			return out
		}
		out.OK = true
		out.Data = map[string]interface{}{"ids": ids, "latestId": latest, "sources": diagnostics}
		return out
	case "metadata":
		if in.VersionID == "" {
			out.Error = "请选择版本。"
			out.ErrorCode = "input"
			return out
		}
		ctx, cancel := context.WithTimeout(in.operationContext(), 150*time.Second)
		defer cancel()
		result, e := b.store.GetVersionMetadata(appstore.GetVersionMetadataInput{Context: ctx, Account: acc, App: app, Platform: platform, VersionID: in.VersionID})
		if e != nil {
			out.ErrorCode, out.Error = storeError(e)
			return out
		}
		out.OK = true
		out.Data = map[string]interface{}{"id": in.VersionID, "version": result.DisplayVersion, "updatedAt": result.ReleaseDate, "sizeBytes": result.SizeBytes, "dateSource": result.DateSource}
		return out
	case "download":
		if in.OutputPath == "" || !filepath.IsAbs(in.OutputPath) || in.VersionID == "" {
			out.Error = "下载路径或版本 ID 无效。"
			out.ErrorCode = "input"
			return out
		}
		bar := progressbar.NewOptions64(-1, progressbar.OptionSetWriter(io.Discard), progressbar.OptionShowBytes(true))
		done := make(chan struct{})
		stopped := make(chan struct{})
		go func() {
			defer close(stopped)
			ticker := time.NewTicker(400 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-done:
					return
				case <-ticker.C:
					state := bar.State()
					percent := state.CurrentPercent
					total := float64(0)
					if math.IsNaN(percent) || math.IsInf(percent, 0) || percent < 0 {
						percent = 0
					}
					if percent > 1 {
						percent = 1
					}
					if percent > 0 {
						total = state.CurrentBytes / percent
					}
					if b.emit != nil {
						b.emit(response{ID: in.ID, Event: "progress", Data: map[string]interface{}{"bytes": int64(state.CurrentBytes), "total": int64(total), "percent": percent, "speed": state.KBsPerSecond * 1024}})
					}
				}
			}
		}()
		result, e := b.store.Download(appstore.DownloadInput{Context: in.operationContext(), Account: acc, App: app, OutputPath: in.OutputPath, Progress: bar, ExternalVersionID: in.VersionID, Platform: platform})
		close(done)
		<-stopped
		if e == nil {
			e = b.store.ReplicateSinf(appstore.ReplicateSinfInput{Sinfs: result.Sinfs, PackagePath: result.DestinationPath})
		}
		if e != nil {
			out.ErrorCode, out.Error = storeError(e)
			return out
		}
		if e = validateIPA(result.DestinationPath, in.Platform, in.VersionID, in.AppID); e != nil {
			out.Error = "IPA 未通过所选应用、版本或设备兼容性校验，请换用其他版本重试。"
			out.ErrorCode = "validation"
			return out
		}
		stat, e := os.Stat(result.DestinationPath)
		if e != nil || stat.Size() == 0 {
			out.Error = "下载文件未完整生成，请重试。"
			out.ErrorCode = "file"
			return out
		}
		out.OK = true
		out.Data = map[string]interface{}{"path": result.DestinationPath, "sizeBytes": stat.Size()}
		return out
	}
	out.Error = "不支持的操作。"
	return out
}
