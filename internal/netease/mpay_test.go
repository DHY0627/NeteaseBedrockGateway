package netease

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"NeteaseBedrockGateway/internal/wplauncher"
)

const (
	testDeviceID  = "dev-1"
	testDeviceKey = "00112233445566778899aabbccddeeff"
)

// recorded 记录一次收到的表单请求。
type recorded struct {
	Path string
	Form url.Values
}

// mockMpay 是一个最简 mpay 服务端，用于在没有真实账号的前提下端到端验证登录流程。
type mockMpay struct {
	mu       sync.Mutex
	requests []recorded
	// smsUpstream 为 true 时 get_sms 返回码 1373（要求用户先上行短信）
	smsUpstream bool
	// smsFailTimes 让 verify_sms 先失败指定次数（模拟上行短信还没生效）
	smsFailTimes int
	smsCalls     int
	server       *httptest.Server
}

func newMockMpay(t *testing.T) *mockMpay {
	t.Helper()
	m := &mockMpay{}
	m.server = httptest.NewServer(http.HandlerFunc(m.handle))
	t.Cleanup(m.server.Close)
	return m
}

func (m *mockMpay) host() string { return m.server.URL }

func (m *mockMpay) record(r *http.Request) {
	_ = r.ParseForm()
	m.mu.Lock()
	m.requests = append(m.requests, recorded{Path: r.URL.Path, Form: r.PostForm})
	m.mu.Unlock()
}

func (m *mockMpay) lastForm(path string) url.Values {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := len(m.requests) - 1; i >= 0; i-- {
		if m.requests[i].Path == path {
			return m.requests[i].Form
		}
	}
	return nil
}

func (m *mockMpay) countPath(path string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, r := range m.requests {
		if r.Path == path {
			n++
		}
	}
	return n
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

func (m *mockMpay) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, `{"code":1,"reason":"method not allowed"}`)
		return
	}
	m.record(r)

	switch r.URL.Path {
	case "/mpay/games/" + defaultProject + "/devices":
		writeJSON(w, 200, `{"device":{"id":"`+testDeviceID+`","key":"`+testDeviceKey+`"}}`)

	case "/mpay/games/" + defaultProject + "/devices/" + testDeviceID + "/users":
		writeJSON(w, 200, `{"user":{"id":"u-email","token":"tok-email","nickname":"邮箱用户"}}`)

	case "/mpay/api/users/login/mobile/get_sms":
		m.mu.Lock()
		upstream := m.smsUpstream
		m.mu.Unlock()
		if upstream {
			writeJSON(w, 400, `{"code":1373,"reason":"请先发送短信","reply_sms":{"content":"FFCSQJYM","number":"106900001234"}}`)
			return
		}
		writeJSON(w, 200, `{"guide_text":"验证码已发送"}`)

	case "/mpay/api/users/login/mobile/verify_sms":
		m.mu.Lock()
		m.smsCalls++
		fail := m.smsCalls <= m.smsFailTimes
		m.mu.Unlock()
		if fail {
			writeJSON(w, 400, `{"code":1300,"reason":"短信尚未送达, 请稍后再试"}`)
			return
		}
		writeJSON(w, 200, `{"ticket":"ticket-sms"}`)

	case "/mpay/api/users/login/mobile/verify_pwd":
		writeJSON(w, 200, `{"ticket":"ticket-pwd"}`)

	case "/mpay/api/users/login/mobile/finish":
		writeJSON(w, 200, `{"user":{"id":"u-phone","token":"tok-phone","nickname":"手机用户"}}`)

	default:
		writeJSON(w, 404, `{"code":404,"reason":"未知路径 `+r.URL.Path+`"}`)
	}
}

func testOptions(m *mockMpay, extra ...Option) []Option {
	opts := []Option{
		WithHost(m.host()),
		WithDeviceCache(NewMemoryDeviceCache()),
	}
	return append(opts, extra...)
}

// 邮箱 + 密码：设备注册 → 加密登录参数 → 组装 sauth。
func TestLoginByEmailMock(t *testing.T) {
	m := newMockMpay(t)

	cookie, err := LoginByEmail(context.Background(), "someone@example.com", "pw123456", testOptions(m)...)
	if err != nil {
		t.Fatalf("LoginByEmail: %v", err)
	}

	// 设备注册只应发生一次
	if n := m.countPath("/mpay/games/" + defaultProject + "/devices"); n != 1 {
		t.Fatalf("设备注册次数 = %d，应为 1", n)
	}

	form := m.lastForm("/mpay/games/" + defaultProject + "/devices/" + testDeviceID + "/users")
	if form == nil {
		t.Fatal("没有收到邮箱登录请求")
	}
	if form.Get("app_channel") != "netease" || form.Get("game_id") != defaultProject {
		t.Errorf("基础参数不对: %v", form)
	}
	if want := b64("someone@example.com"); form.Get("un") != want {
		t.Errorf("un = %q，应为 %q", form.Get("un"), want)
	}
	// params 必须能用设备密钥解回明文
	plain, err := aesECBPKCS7Decrypt(form.Get("params"), mustKey(t, testDeviceKey))
	if err != nil {
		t.Fatalf("params 解密失败: %v", err)
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.Unmarshal(plain, &body); err != nil {
		t.Fatalf("params 不是 JSON: %v", err)
	}
	if body.Username != "someone@example.com" || body.Password != md5hex("pw123456") {
		t.Errorf("params 内容不对: %s", plain)
	}

	// sauth 关键字段
	assertSauth(t, cookie, map[string]string{
		"gameid":        "x19",
		"app_channel":   "netease",
		"login_channel": "netease",
		"platform":      "pc",
		"sdk_version":   defaultSDKVersion,
		"sdkuid":        "u-email",
		"sessionid":     "tok-email",
		"deviceid":      testDeviceID,
		"timestamp":     "null",
		"userid":        "null",
	})
	if got := len(fieldString(t, cookie, "udid")); got != 16 {
		t.Errorf("udid 长度 = %d，应为 16", got)
	}
	if got := len(fieldString(t, cookie, "client_login_sn")); got != 16 {
		t.Errorf("client_login_sn 长度 = %d，应为 16", got)
	}
}

// 手机号 + 密码：verify_pwd 拿 ticket → finish 换 user。
func TestLoginByPhonePasswordMock(t *testing.T) {
	m := newMockMpay(t)

	cookie, err := LoginByPhonePassword(context.Background(), "13800000000", "pw123456", testOptions(m)...)
	if err != nil {
		t.Fatalf("LoginByPhonePassword: %v", err)
	}
	if fieldString(t, cookie, "sdkuid") != "u-phone" || fieldString(t, cookie, "sessionid") != "tok-phone" {
		t.Fatalf("sauth 内容不对: %s", cookie.RawJSON())
	}

	form := m.lastForm("/mpay/api/users/login/mobile/verify_pwd")
	if form == nil {
		t.Fatal("没有收到 verify_pwd 请求")
	}
	if form.Get("password") != md5hex("pw123456") {
		t.Errorf("password 应为 md5: %q", form.Get("password"))
	}
	if form.Get("mobile") != "13800000000" || form.Get("device_id") != testDeviceID {
		t.Errorf("verify_pwd 参数不对: %v", form)
	}

	finish := m.lastForm("/mpay/api/users/login/mobile/finish")
	if finish == nil || finish.Get("ticket") != "ticket-pwd" {
		t.Errorf("finish 没有带上 ticket: %v", finish)
	}
}

// 手机号 + 短信（下行验证码）。
func TestLoginByPhoneSmsMock(t *testing.T) {
	m := newMockMpay(t)

	var got *SmsChallenge
	cookie, err := LoginByPhoneSms(context.Background(), "13800000000",
		testOptions(m, WithSmsCodeHandler(func(ch *SmsChallenge) (string, error) {
			got = ch
			return "654321", nil
		}))...)
	if err != nil {
		t.Fatalf("LoginByPhoneSms: %v", err)
	}
	if got == nil || got.Upstream {
		t.Fatalf("回调拿到的 challenge 不对: %+v", got)
	}

	form := m.lastForm("/mpay/api/users/login/mobile/verify_sms")
	if form == nil {
		t.Fatal("没有收到 verify_sms 请求")
	}
	if form.Get("smscode") != "654321" {
		t.Errorf("smscode 未透传: %v", form)
	}
	if form.Get("up_content") != "" {
		t.Errorf("下行验证码不应带 up_content: %v", form)
	}
	if fieldString(t, cookie, "sdkuid") != "u-phone" {
		t.Errorf("sauth 未生成: %s", cookie.RawJSON())
	}
}

// 手机号 + 短信（网易要求先上行短信），并且上行短信要等一会儿才生效。
func TestLoginByPhoneSmsUpstreamMock(t *testing.T) {
	m := newMockMpay(t)
	m.smsUpstream = true
	m.smsFailTimes = 2 // 前两次轮询失败，第三次成功

	var challenge *SmsChallenge
	calls := 0
	cookie, err := LoginByPhoneSms(context.Background(), "13800000000",
		testOptions(m,
			WithSmsPolling(5*time.Millisecond, 10),
			WithSmsCodeHandler(func(ch *SmsChallenge) (string, error) {
				challenge = ch
				calls++
				return "", nil // 上行短信：返回空串表示「我已经发过了」
			}))...)
	if err != nil {
		t.Fatalf("LoginByPhoneSms(上行短信): %v", err)
	}
	if challenge == nil || !challenge.Upstream {
		t.Fatalf("应识别为上行短信场景: %+v", challenge)
	}
	if challenge.Content != "FFCSQJYM" || challenge.Number != "106900001234" {
		t.Errorf("上行短信内容解析不对: %+v", challenge)
	}
	if calls != 1 {
		t.Errorf("验证码回调应只调用一次，实际 %d", calls)
	}
	if n := m.countPath("/mpay/api/users/login/mobile/verify_sms"); n != 3 {
		t.Errorf("verify_sms 轮询次数 = %d，应为 3", n)
	}
	last := m.lastForm("/mpay/api/users/login/mobile/verify_sms")
	if last.Get("up_content") != "FFCSQJYM" {
		t.Errorf("轮询应带 up_content: %v", last)
	}
	if fieldString(t, cookie, "sdkuid") != "u-phone" {
		t.Errorf("sauth 未生成: %s", cookie.RawJSON())
	}
}

// 短信方式没有验证码回调时必须给出明确错误。
func TestLoginByPhoneSmsRequiresHandler(t *testing.T) {
	m := newMockMpay(t)
	_, err := LoginByPhoneSms(context.Background(), "13800000000", testOptions(m)...)
	if err == nil || !strings.Contains(err.Error(), "验证码回调") {
		t.Fatalf("应提示需要验证码回调，实际: %v", err)
	}
}

// 上行短信轮询耗尽后要报错，而不是静默返回空 ticket。
func TestLoginByPhoneSmsUpstreamTimeout(t *testing.T) {
	m := newMockMpay(t)
	m.smsUpstream = true
	m.smsFailTimes = 100

	_, err := LoginByPhoneSms(context.Background(), "13800000000",
		testOptions(m, WithSmsPolling(time.Millisecond, 3),
			WithSmsCodeHandler(func(*SmsChallenge) (string, error) { return "", nil }))...)
	if err == nil || !strings.Contains(err.Error(), "上行短信验证超时") {
		t.Fatalf("应提示上行短信超时，实际: %v", err)
	}
}

// 设备注册失败要能如实报错。
func TestLoginByEmailDeviceRegisterFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 400, `{"code":1001,"reason":"设备注册被风控"}`)
	}))
	defer srv.Close()

	_, err := LoginByEmail(context.Background(), "a@b.com", "pw",
		WithHost(srv.URL), WithDeviceCache(NewMemoryDeviceCache()))
	if err == nil || !strings.Contains(err.Error(), "设备注册被风控") {
		t.Fatalf("应报设备注册失败，实际: %v", err)
	}
}

// 邮箱登录返回缺少 user 的响应时要报错。
func TestLoginByEmailMissingUser(t *testing.T) {
	m := newMockMpay(t)
	m.server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/devices") {
			writeJSON(w, 200, `{"device":{"id":"`+testDeviceID+`","key":"`+testDeviceKey+`"}}`)
			return
		}
		writeJSON(w, 200, `{"user":{}}`)
	})

	_, err := LoginByEmail(context.Background(), "a@b.com", "pw", testOptions(m)...)
	if err == nil || !strings.Contains(err.Error(), "响应中没有有效的 user") {
		t.Fatalf("应报缺少 user，实际: %v", err)
	}
}

// LoginBySauth 直接使用已有登录态，不发任何网络请求。
func TestLoginBySauth(t *testing.T) {
	raw := `{"gameid":"x19","sdkuid":"u9","sessionid":"tok9","platform":"ad"}`
	cookie, err := LoginBySauth(raw)
	if err != nil {
		t.Fatalf("LoginBySauth: %v", err)
	}
	if fieldString(t, cookie, "sdkuid") != "u9" {
		t.Fatalf("透传失败: %s", cookie.RawJSON())
	}
	// 缺失字段应被补上（app_channel / sdk_version / udid 等）
	for _, k := range []string{"app_channel", "login_channel", "sdk_version", "aim_info", "udid", "client_login_sn", "gameid", "platform"} {
		if !hasField(t, cookie, k) {
			t.Errorf("EnsureSauthDefaults 未补齐 %s: %s", k, cookie.RawJSON())
		}
	}
	// AuthJSON 里 platform 必须已换成 pc
	if !strings.Contains(cookie.SauthJSON(), `"platform":"pc"`) {
		t.Errorf("platform 未转换为 pc: %s", cookie.SauthJSON())
	}

	if _, err := LoginBySauth("not json"); err == nil {
		t.Fatal("非法 sauth 应当报错")
	}
	if _, err := LoginBySauth("   "); err == nil {
		t.Fatal("空 sauth 应当报错")
	}
}

func mustKey(t *testing.T, hexKey string) []byte {
	t.Helper()
	d := &Device{Key: hexKey}
	k, err := d.aesKey()
	if err != nil {
		t.Fatalf("aesKey: %v", err)
	}
	return k
}

func b64(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

// rawFields 把 sauth 登录态解析成字段表（测试只读，不改变原始字段顺序）。
func rawFields(t *testing.T, c wplauncher.Sauth) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(c.RawJSON()), &m); err != nil {
		t.Fatalf("sauth 不是合法 JSON: %v (%s)", err, c.RawJSON())
	}
	return m
}

// fieldString 取 sauth 字段的字符串值（对象/数组原样输出为 JSON 文本）。
func fieldString(t *testing.T, c wplauncher.Sauth, key string) string {
	t.Helper()
	v, ok := rawFields(t, c)[key]
	if !ok {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func hasField(t *testing.T, c wplauncher.Sauth, key string) bool {
	t.Helper()
	_, ok := rawFields(t, c)[key]
	return ok
}

func assertSauth(t *testing.T, cookie wplauncher.Sauth, want map[string]string) {
	t.Helper()
	for k, v := range want {
		if got := fieldString(t, cookie, k); got != v {
			t.Errorf("sauth[%s] = %q，应为 %q", k, got, v)
		}
	}
}
