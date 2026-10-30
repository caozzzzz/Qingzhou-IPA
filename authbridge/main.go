package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/majd/ipatool/v2/pkg/appstore"
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
	ID             int           `json:"id"`
	Action         string        `json:"action"`
	Email          string        `json:"email"`
	Password       string        `json:"password"`
	Code           string        `json:"code"`
	GUID           string        `json:"guid"`
	Session        *savedSession `json:"session"`
	Query          string        `json:"query"`
	Country        string        `json:"country"`
	Platform       string        `json:"platform"`
	AppID          int64         `json:"appId"`
	VersionID      string        `json:"versionId"`
	OutputPath     string        `json:"outputPath"`
	AcquireLicense bool          `json:"acquireLicense"`
	Source         string        `json:"source"`
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

// Never return upstream response metadata, request bodies, cookies or tokens as errors.
func authenticationError(err error) (string, string) {
	s := strings.ToLower(err.Error())
	switch {
	case strings.Contains(s, "password"), strings.Contains(s, "credentials"), strings.Contains(s, "incorrect"), strings.Contains(s, "invalid credential"):
		return "credentials", "Apple 未接受登录信息，请核对账号、密码及验证码。"
	case strings.Contains(s, "locked"), strings.Contains(s, "disabled"):
		return "locked", "Apple 账户可能被锁定或停用，请先在 Apple 官方渠道检查账户。"
	case strings.Contains(s, "timeout"), strings.Contains(s, "deadline"), strings.Contains(s, "dial tcp"), strings.Contains(s, "no such host"), strings.Contains(s, "connection"), strings.Contains(s, "tls"):
		return "network", "连接 Apple 服务失败或超时，请检查网络后重试。"
	case strings.Contains(s, "sap"), strings.Contains(s, "signer"), strings.Contains(s, "bag"):
		return "service", "无法准备 App Store 认证环境，请检查网络，稍后重试。"
	default:
		return "apple", "Apple 未完成此次认证。请确认账户可正常使用 App Store，再重试；也可能是服务或认证协议发生变化。"
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
