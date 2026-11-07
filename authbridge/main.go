package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/majd/ipatool/v2/pkg/appstore"
	storehttp "github.com/majd/ipatool/v2/pkg/http"
	"github.com/majd/ipatool/v2/pkg/util/operatingsystem"
)

// No disk keychain: only the Electron main process may persist an encrypted session.
type memoryKeychain struct{ values map[string][]byte }

func (k *memoryKeychain) Get(key string) ([]byte, error) {
	v, ok := k.values[key]
	if !ok {
		return nil, errors.New("missing key")
	}
	return append([]byte(nil), v...), nil
}
func (k *memoryKeychain) Set(key string, value []byte) error {
	if key == "account" {
		var account appstore.Account
		if err := json.Unmarshal(value, &account); err != nil {
			return err
		}
		account.Password = ""
		var err error
		value, err = json.Marshal(account)
		if err != nil {
			return err
		}
	}
	k.values[key] = append([]byte(nil), value...)
	return nil
}
func (k *memoryKeychain) Remove(key string) error { delete(k.values, key); return nil }

type cookieRecord struct {
	URL    string       `json:"url"`
	Cookie *http.Cookie `json:"cookie"`
}
type sessionJar struct {
	jar     *cookiejar.Jar
	mu      sync.Mutex
	records map[string]cookieRecord
}

func newJar() *sessionJar {
	jar, _ := cookiejar.New(nil)
	return &sessionJar{jar: jar, records: map[string]cookieRecord{}}
}
func (j *sessionJar) Cookies(u *url.URL) []*http.Cookie { return j.jar.Cookies(u) }
func (j *sessionJar) Save() error                       { return nil }
func (j *sessionJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, c := range cookies {
		copy := *c
		// Convert relative lifetime once, so restoring a session never extends it.
		if copy.MaxAge > 0 {
			copy.Expires = time.Now().Add(time.Duration(copy.MaxAge) * time.Second)
			copy.MaxAge = 0
		}
		key := u.Hostname() + "|" + copy.Domain + "|" + copy.Path + "|" + copy.Name
		j.records[key] = cookieRecord{URL: u.String(), Cookie: &copy}
		j.jar.SetCookies(u, []*http.Cookie{&copy})
	}
}
func (j *sessionJar) snapshot() []cookieRecord {
	j.mu.Lock()
	defer j.mu.Unlock()
	result := []cookieRecord{}
	for _, record := range j.records {
		if record.Cookie.MaxAge < 0 || (!record.Cookie.Expires.IsZero() && record.Cookie.Expires.Before(time.Now())) {
			continue
		}
		result = append(result, record)
	}
	return result
}

type localMachine struct{ guid string }

func (m localMachine) MacAddress() (string, error) {
	if len(m.guid) != 12 {
		return "", errors.New("invalid local identity")
	}
	parts := []string{}
	for i := 0; i < 12; i += 2 {
		parts = append(parts, m.guid[i:i+2])
	}
	addr, err := net.ParseMAC(strings.Join(parts, ":"))
	if err != nil {
		return "", err
	}
	return addr.String(), nil
}
func (localMachine) HomeDirectory() string { p, _ := os.UserHomeDir(); return p }
func (localMachine) ReadPassword(int) ([]byte, error) {
	return nil, errors.New("terminal input disabled")
}

type savedSession struct {
	Account appstore.Account `json:"account"`
	Cookies []cookieRecord   `json:"cookies"`
}
type request struct {
	Context        context.Context `json:"-"`
	ID             int             `json:"id"`
	Action         string          `json:"action"`
	Email          string          `json:"email"`
	Password       string          `json:"password"`
	Code           string          `json:"code"`
	GUID           string          `json:"guid"`
	Session        *savedSession   `json:"session"`
	Query          string          `json:"query"`
	Country        string          `json:"country"`
	Platform       string          `json:"platform"`
	AppID          int64           `json:"appId"`
	VersionID      string          `json:"versionId"`
	OutputPath     string          `json:"outputPath"`
	AcquireLicense bool            `json:"acquireLicense"`
	Source         string          `json:"source"`
	Transport      string          `json:"transport"`
	PathMode       string          `json:"pathMode"`
}

func (in request) operationContext() context.Context {
	if in.Context != nil {
		return in.Context
	}
	return context.Background()
}

type publicAccount struct {
	Email      string `json:"email"`
	Name       string `json:"name"`
	Storefront string `json:"storefront"`
}
type response struct {
	ID        int            `json:"id"`
	OK        bool           `json:"ok"`
	Need2FA   bool           `json:"need2FA,omitempty"`
	Error     string         `json:"error,omitempty"`
	ErrorCode string         `json:"errorCode,omitempty"`
	Account   *publicAccount `json:"account,omitempty"`
	Session   *savedSession  `json:"session,omitempty"`
	Data      interface{}    `json:"data,omitempty"`
	Event     string         `json:"event,omitempty"`
}
type bridge struct {
	store    appstore.AppStore
	keychain *memoryKeychain
	jar      *sessionJar
	guid     string
	emit     func(response)
}

func (b *bridge) reset(guid string) {
	b.keychain = &memoryKeychain{values: map[string][]byte{}}
	b.jar = newJar()
	b.guid = guid
	b.store = appstore.NewAppStore(appstore.Args{Keychain: b.keychain, CookieJar: b.jar, Machine: localMachine{guid}, OperatingSystem: operatingsystem.New()})
}
func public(acc appstore.Account) *publicAccount {
	return &publicAccount{acc.Email, acc.Name, acc.StoreFront}
}

func safeAuthenticationDiagnostics(e *storehttp.UnexpectedResponseError) string {
	network, method, path, kind := "unknown", "unknown", "unknown", "other"
	switch e.Protocol {
	case "HTTP/1.1", "HTTP/2.0", "URLSession":
		network = e.Protocol
	}
	switch e.Method {
	case "GET", "POST":
		method = e.Method
	}
	switch e.AuthPath {
	case "AUTH_BARE", "AUTH_SLASH":
		path = e.AuthPath
	}
	contentType := strings.ToLower(strings.TrimSpace(strings.SplitN(e.ContentType, ";", 2)[0]))
	switch contentType {
	case "text/html":
		kind = "HTML"
	case "text/xml", "application/xml", "application/x-plist":
		kind = "XML"
	case "":
		kind = "empty"
	}
	length := e.BodyLength
	if length < 0 {
		length = 0
	}
	if length > 2<<20 {
		length = 2 << 20
	}
	host := "unknown"
	switch e.AuthHost {
	case "BUY", "POD", "OTHER":
		host = e.AuthHost
	}
	return fmt.Sprintf("网络 %s；请求 %s；主机 %s；路径 %s；响应 %s；%d 字节。", network, method, host, path, kind, length)
}

var authenticationAttemptsPattern = regexp.MustCompile(`after (\d) attempts \(((?:HTTP )?(?:\d{3}|transport error)(?:, (?:HTTP )?(?:\d{3}|transport error))*)\)`)

// authenticationContext reports only allowlisted retry and routing facts parsed
// from the internal error chain: attempt count, HTTP statuses and whether the
// failure happened after a Store pod redirect.
func authenticationContext(err error) string {
	message := err.Error()
	parts := []string{}
	if match := authenticationAttemptsPattern.FindStringSubmatch(message); match != nil {
		parts = append(parts, fmt.Sprintf("尝试 %s 次（%s）", match[1], match[2]))
	}
	if strings.Contains(message, "at Store pod") {
		parts = append(parts, "已跳转到分区服务器")
	} else {
		parts = append(parts, "未发生跳转")
	}
	parts = append(parts, "通道 "+storehttp.AuthenticationTransport(), "端点 "+appstore.AuthenticationPathMode())
	return strings.Join(parts, "；") + "。"
}

// Never return upstream response metadata, request bodies, cookies or tokens as errors.
func authenticationError(err error) (string, string) {
	var responseError *storehttp.UnexpectedResponseError
	if errors.As(err, &responseError) && responseError.StatusCode >= 100 && responseError.StatusCode <= 599 {
		status := responseError.StatusCode
		details := safeAuthenticationDiagnostics(responseError) + authenticationContext(err)
		switch status {
		case 301, 302, 307, 308:
			if responseError.Reason == "authentication redirect is missing Location" {
				return "auth_redirect_missing", fmt.Sprintf("Apple 认证响应缺少跳转地址，无法继续登录（HTTP %d；REDIRECT_NO_LOCATION）。%s", status, details)
			}
			return "auth_redirect", fmt.Sprintf("Apple 认证跳转未完成（HTTP %d）。%s", status, details)
		case 401:
			return "auth_http", "Apple 未接受登录请求（HTTP 401）。请核对密码；若开启双重认证，可展开手动验证码入口。" + details
		case 403, 404:
			return "auth_http", fmt.Sprintf("Apple 登录接口未返回可用认证数据（HTTP %d）。这不等于密码错误。%s", status, details)
		case 429:
			return "auth_rate", "Apple 暂时限制了登录请求（HTTP 429），请稍后再试，避免连续尝试。" + details
		default:
			return "auth_http", fmt.Sprintf("Apple 登录接口返回异常（HTTP %d）。%s", status, details)
		}
	}
	var failure *appstore.AuthenticationFailure
	var transportError *storehttp.TransportError
	if errors.As(err, &transportError) {
		reason := "NETWORK"
		var nativeError *storehttp.NativeAuthenticationNetworkError
		if errors.As(err, &nativeError) && nativeError.Code < 0 && nativeError.Code >= -2000 {
			reason = fmt.Sprintf("URLSession %d", nativeError.Code)
		}
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			reason = "EOF"
		}
		var networkError net.Error
		if errors.As(err, &networkError) && networkError.Timeout() {
			reason = "TIMEOUT"
		}
		return "auth_transport", fmt.Sprintf("连接 Apple 登录接口时中断（%s）。认证环境通过不代表登录连接也通过，可切换 Wi-Fi／蜂窝网络后对照此错误码。", reason)
	}
	if errors.As(err, &failure) {
		if failure.Disabled {
			return "locked", "Apple 返回账户停用提示，请先在官方 App Store 检查账户。"
		}
		if failure.Verification {
			return "verification", "Apple 未接受双重验证码，请使用受信任设备上的新六位验证码重试。"
		}
		if failure.FailureType == appstore.FailureTypeInvalidCredentials {
			return "credentials", "Apple 未接受登录凭据（代码 -5000）。请使用 Apple 账户密码；若已开启双重认证，可展开手动验证码入口。"
		}
		return "auth_response", fmt.Sprintf("Apple 未完成登录（HTTP %d，Apple 代码 %s）。请记录此错误码及登录环境检查结果。", failure.StatusCode, failure.FailureType)
	}
	s := strings.ToLower(err.Error())
	switch {
	case strings.Contains(s, "failed to get storefront header"), strings.Contains(s, "failed to get pod header"):
		return "auth_headers", "Apple 响应缺少商店信息，未保存登录状态（HEADERS）。"
	case strings.Contains(s, "cache apple sap"), strings.Contains(s, "find user cache"):
		return "auth_cache", "无法保存认证组件，请检查应用缓存目录和可用空间（CACHE）。"
	case strings.Contains(s, "load apple sap assets"), strings.Contains(s, "download apple sap assets"), strings.Contains(s, "open apple software update"):
		return "auth_assets", "认证组件下载或校验失败，请确认当前网络能够访问 Apple 软件更新服务后重试（ASSETS）。"
	case strings.Contains(s, "failed to get bag"):
		return "auth_bag", "无法获取 App Store 认证配置，请切换网络后重试（BAG）。"
	case strings.Contains(s, "unicorn"), strings.Contains(s, "execute sap guest"), strings.Contains(s, "initialize apple sap"):
		return "auth_engine", "本机认证解释器执行失败，请点击“检查登录环境”查看是否可用（ENGINE）。"
	case strings.Contains(s, "password"), strings.Contains(s, "credentials"), strings.Contains(s, "incorrect"), strings.Contains(s, "invalid credential"):
		return "credentials", "Apple 未接受登录信息，请核对账号、密码及验证码。"
	case strings.Contains(s, "locked"), strings.Contains(s, "disabled"):
		return "locked", "Apple 账户可能被锁定或停用，请先在 Apple 官方渠道检查账户。"
	case strings.Contains(s, "timeout"), strings.Contains(s, "deadline"), strings.Contains(s, "dial tcp"), strings.Contains(s, "no such host"), strings.Contains(s, "connection"), strings.Contains(s, "tls"):
		return "network", "连接 Apple 服务失败或超时，请检查网络后重试。"
	case strings.Contains(s, "sap"), strings.Contains(s, "signer"), strings.Contains(s, "bag"):
		return "service", "无法准备 App Store 认证环境，请检查网络，稍后重试。"
	default:
		return "apple", "Apple 未完成此次认证（AUTH_UNKNOWN）。请记录登录环境检查结果；当前信息不足以判断是账户还是协议问题。"
	}
}
func (b *bridge) handle(in request) response {
	out := response{ID: in.ID}
	if in.Action == "ping" {
		out.OK = true
		return out
	}
	if _, err := (localMachine{in.GUID}).MacAddress(); err != nil {
		out.Error = "本机认证标识无效。"
		out.ErrorCode = "identity"
		return out
	}
	if b.store == nil || b.guid != in.GUID {
		b.reset(in.GUID)
	}
	switch in.Action {
	case "checkAuth":
		if err := appstore.QingzhouCheckAuthentication(b.store); err != nil {
			out.ErrorCode, out.Error = authenticationError(err)
			return out
		}
		out.OK = true
		out.Data = map[string]string{"message": "认证配置、组件和签名检查通过。此检查未使用 Apple 账号或密码。"}
		return out
	case "search":
		data, err := catalogSearch(in.Query, in.Country, in.Platform)
		if err != nil {
			out.ErrorCode = "catalog"
			out.Error = "无法查询 Apple 应用目录，请检查网络、商店地区或搜索内容后重试。"
			return out
		}
		out.OK = true
		out.Data = data
		return out
	case "export":
		acc, err := b.store.AccountInfo()
		if err != nil {
			out.Error = "请先登录 Apple 账户。"
			out.ErrorCode = "login"
			return out
		}
		acc.Account.Password = ""
		out.OK = true
		out.Session = &savedSession{Account: acc.Account, Cookies: b.jar.snapshot()}
		return out
	case "versions", "metadata", "download":
		return b.storeOperation(in)
	case "login":
		if strings.TrimSpace(in.Email) == "" || in.Password == "" || len(in.Email) > 254 || len(in.Password) > 4096 {
			out.Error = "请输入有效的 Apple 账号和密码。"
			out.ErrorCode = "input"
			return out
		}
		if in.Code != "" {
			if len(in.Code) != 6 || strings.IndexFunc(in.Code, func(r rune) bool { return r < '0' || r > '9' }) != -1 {
				out.Error = "验证码必须是六位数字。"
				out.ErrorCode = "input"
				return out
			}
		}
		storehttp.SetAuthenticationTransport(in.Transport)
		appstore.SetAuthenticationPathMode(in.PathMode)
		result, err := b.store.Login(appstore.LoginInput{Email: strings.TrimSpace(in.Email), Password: in.Password, AuthCode: in.Code})
		in.Password = ""
		if err != nil {
			if errors.Is(err, appstore.ErrAuthCodeRequired) {
				out.Need2FA = true
				return out
			}
			out.ErrorCode, out.Error = authenticationError(err)
			return out
		}
		result.Account.Password = ""
		out.OK = true
		out.Account = public(result.Account)
		out.Session = &savedSession{Account: result.Account, Cookies: b.jar.snapshot()}
		return out
	case "restore":
		if in.Session == nil || in.Session.Account.Email == "" || in.Session.Account.PasswordToken == "" {
			out.Error = "保存的登录状态无效，请重新登录。"
			out.ErrorCode = "session"
			return out
		}
		b.reset(in.GUID)
		in.Session.Account.Password = ""
		data, _ := json.Marshal(in.Session.Account)
		_ = b.keychain.Set("account", data)
		for _, record := range in.Session.Cookies {
			u, err := url.Parse(record.URL)
			if err != nil || u.Scheme != "https" || record.Cookie == nil {
				continue
			}
			if u.Hostname() != "apple.com" && !strings.HasSuffix(u.Hostname(), ".apple.com") {
				continue
			}
			b.jar.SetCookies(u, []*http.Cookie{record.Cookie})
		}
		out.OK = true
		out.Account = public(in.Session.Account)
		return out
	case "logout":
		b.reset(in.GUID)
		out.OK = true
		return out
	default:
		out.Error = "不支持的认证操作。"
		out.ErrorCode = "operation"
		return out
	}
}
func run(r io.Reader, w io.Writer) {
	decoder := json.NewDecoder(bufio.NewReader(r))
	encoder := json.NewEncoder(w)
	var mu sync.Mutex
	emit := func(out response) { mu.Lock(); defer mu.Unlock(); _ = encoder.Encode(out) }
	b := &bridge{emit: emit}
	for {
		var in request
		err := decoder.Decode(&in)
		if err == io.EOF {
			return
		}
		if err != nil {
			emit(response{Error: "认证请求格式无效。", ErrorCode: "protocol"})
			return
		}
		out := b.handle(in)
		in.Password = ""
		emit(out)
	}
}
func main() {
	defer func() {
		if recover() != nil {
			fmt.Fprintln(os.Stderr, "authentication bridge failed")
			os.Exit(1)
		}
	}()
	run(os.Stdin, os.Stdout)
}
