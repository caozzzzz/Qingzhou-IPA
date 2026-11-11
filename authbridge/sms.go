package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strings"
	"time"

	"qingzhou.local/authbridge/smsauth"
)

type storeChallenge struct {
	email, attempt string
	expires        time.Time
}

var smsAttemptPattern = regexp.MustCompile(`^[0-9A-Fa-f]{8}(?:-[0-9A-Fa-f]{4}){3}-[0-9A-Fa-f]{12}$`)

func validSMSAttempt(attempt string) bool {
	return smsAttemptPattern.MatchString(attempt)
}

func (b *bridge) clearStoreChallenge() {
	b.storeChallenge = nil
	b.sms = nil
}

func (b *bridge) matchesStoreChallenge(in request) bool {
	pending := b.storeChallenge
	return runtime.GOOS == "ios" && pending != nil && pending.attempt == in.Attempt &&
		time.Now().Before(pending.expires) && strings.EqualFold(pending.email, strings.TrimSpace(in.Email))
}

func (b *bridge) handleSMS(in request, out response) response {
	if in.Action == "smsCancel" {
		if b.storeChallenge != nil && b.storeChallenge.attempt == in.Attempt {
			b.clearStoreChallenge()
		}
		out.OK = true
		return out
	}
	if !b.matchesStoreChallenge(in) {
		return smsFailure(out, &smsauth.Error{Stage: "SMS_EXPIRED"})
	}
	ctx, cancel := context.WithTimeout(in.operationContext(), 110*time.Second)
	defer cancel()
	switch in.Action {
	case "smsPrepare":
		if !b.sms.Active() {
			if in.Password == "" || len(in.Password) > 4096 {
				return smsFailure(out, &smsauth.Error{Stage: "SMS_CREDENTIALS"})
			}
			if b.smsClient == nil {
				b.smsClient = smsauth.New(os.Getenv("QINGZHOU_CACHE_DIR"))
			}
			session, err := b.smsClient.Begin(ctx, b.storeChallenge.email, in.Password, b.storeChallenge.expires)
			if err != nil {
				return smsFailure(out, err)
			}
			b.sms = session
		}
	case "smsSend":
		if !b.sms.Active() {
			return smsFailure(out, &smsauth.Error{Stage: "SMS_EXPIRED"})
		}
		if err := b.sms.Send(ctx, in.PhoneID); err != nil {
			out.Data = b.sms.State()
			return smsFailure(out, err)
		}
	}
	out.OK = true
	out.Data = b.sms.State()
	return out
}

func smsFailure(out response, err error) response {
	out.ErrorCode = "SMS_NETWORK"
	out.Error = "短信验证连接失败，请检查网络后重试（SMS_NETWORK）。"
	if errors.Is(err, context.Canceled) {
		out.ErrorCode = "canceled"
		out.Error = "已取消。"
		return out
	}
	var failure *smsauth.Error
	if !errors.As(err, &failure) {
		return out
	}
	out.ErrorCode = failure.Stage
	switch failure.Stage {
	case "SMS_EXPIRED":
		out.Error = "短信验证会话已过期，请重新登录。"
	case "SMS_CREDENTIALS":
		out.Error = "请重新输入 Apple 账户密码。"
	case "SMS_CONFIG":
		out.Error = "无法获取 Apple 短信验证配置。"
	case "SMS_DEVICE_PROVISION", "SMS_DEVICE_PROOF":
		out.Error = "短信验证所需的设备证明未准备完成。"
	case "SMS_CHALLENGE":
		out.Error = "Apple 没有为这次登录提供可用的双重验证会话。"
	case "SMS_PHONE_LIST":
		out.Error = "Apple 未提供可用于此次登录的受信任手机号。"
	case "SMS_SEND":
		out.Error = "Apple 未确认受理短信请求。"
	case "SMS_VERIFY":
		out.Error = "Apple 未接受本次短信验证码，请检查验证码并重试。"
	case "SMS_SEND_LIMIT":
		out.Error = "本次验证已达到短信请求次数上限。"
	case "SMS_VERIFY_LIMIT":
		out.Error = "本次验证已达到验证码尝试次数上限，请重新登录。"
	case "SMS_COOLDOWN":
		out.Error = "请等待倒计时结束后重发。"
	case "SMS_CODE":
		out.Error = "请先请求短信，再输入六位验证码。"
	default:
		out.Error = "Apple 短信认证未完成。"
	}
	out.Error += "（" + failure.Stage
	if failure.HTTP >= 100 && failure.HTTP <= 599 {
		out.Error += fmt.Sprintf("，HTTP %d", failure.HTTP)
	}
	if failure.Code != 0 {
		out.Error += fmt.Sprintf("，Apple 代码 %d", failure.Code)
	}
	out.Error += "）"
	return out
}
