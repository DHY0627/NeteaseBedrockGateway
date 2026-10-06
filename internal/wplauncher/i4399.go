package wplauncher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const i4399Base = "https://ptlogin.4399.com"
const i4399OauthURL = "https://m.4399api.com/openapiv2/oauth.html"

func newCookieJar() (*cookiejar.Jar, error) {
	return cookiejar.New(nil)
}

// I4399Config 对应 Kotlin 端 Config
type I4399Config struct {
	GameKey     string
	GameVersion string
	BID         string
}

// ConfigNetEaseMC 对应 Kotlin 端 CONFIG_NET_EASE_MC
var ConfigNetEaseMC = I4399Config{
	GameKey:     "115716",
	GameVersion: "3.6.29.285648",
	BID:         "com.netease.mc.m4399",
}

func (c I4399Config) generateDeviceRaw(device string) string {
	return fmt.Sprintf(`{"GAME_KEY":%q,"GAME_VERSION":%q,"BID":%q,"DEVICE_IDENTIFIER":%q}`,
		c.GameKey, c.GameVersion, c.BID, device)
}

// I4399Client 4399 第三方 OAuth 客户端 (对应 Kotlin 端 I4399GameSDKAPI)
type I4399Client struct {
	Client  *http.Client
	Config  I4399Config
	Session map[string]string // OAuth 会话中的隐藏表单字段

	OnCaptcha  func(image []byte) (string, error)
	OnRealName func() (name, idCard string, err error)
}

// NewI4399Client 创建 4399 客户端 (内置 cookie jar, 自动跟随 GET 重定向)
func NewI4399Client(cfg I4399Config) (*I4399Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			// 复刻 ktor: POST 不自动跟随重定向, 由调用方手动处理
			if len(via) > 0 && via[len(via)-1].Method == http.MethodPost {
				return http.ErrUseLastResponse
			}
			return nil
		},
		Timeout: 30 * time.Second,
	}
	return &I4399Client{Client: client, Config: cfg}, nil
}

// registerDevice 复刻 I4399GameSDKAPI.registerDevice
// 生成 OAuth url -> 访问登录页 -> 解析隐藏表单字段作为 OAuth 会话
func (c *I4399Client) registerDevice(ctx context.Context, device string) error {
	form := url.Values{}
	form.Set("usernames", "")
	form.Set("top_bar", "1")
	form.Set("state", "")
	form.Set("device", c.Config.generateDeviceRaw(device))

	body, _, err := c.doForm(ctx, http.MethodPost, i4399OauthURL, form)
	if err != nil {
		return err
	}

	var oauth struct {
		Code   int `json:"code"`
		Result struct {
			LoginURL string `json:"login_url"`
		} `json:"result"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &oauth); err != nil {
		return fmt.Errorf("解析 oauth.html 响应失败: %w", err)
	}
	if oauth.Result.LoginURL == "" {
		return fmt.Errorf("获取 login_url 失败: code=%d message=%s", oauth.Code, oauth.Message)
	}

	html, err := c.doGetText(ctx, oauth.Result.LoginURL)
	if err != nil {
		return err
	}

	c.Session = decodeForms(html, defaultBefore)
	if len(c.Session) == 0 {
		return errors.New("解析 OAuth 会话表单失败 (未找到隐藏字段)")
	}
	return nil
}

const defaultBefore = `<label for="protocol">我已同意</label>`

// decodeForms 复刻 Kotlin decodeForms:
// 截取 substringBefore 之前的内容, 提取所有 type="hidden" 的 input 的 name/value
func decodeForms(html, substringBefore string) map[string]string {
	if substringBefore != "" {
		if idx := strings.Index(html, substringBefore); idx >= 0 {
			html = html[:idx]
		}
	}

	forms := map[string]string{}
	for _, tag := range inputTagRe.FindAllString(html, -1) {
		if !strings.Contains(tag, `type="hidden"`) {
			continue
		}
		m := nameAttrRe.FindStringSubmatch(tag)
		if m == nil {
			continue
		}
		v := valueAttrRe.FindStringSubmatch(tag)
		if v == nil {
			continue
		}
		forms[m[1]] = v[1]
	}
	return forms
}

var (
	inputTagRe  = regexp.MustCompile(`(?i)<input\b[^>]*>`)
	nameAttrRe  = regexp.MustCompile(`(?i)\bname\s*=\s*"([0-9a-zA-Z_]+)"`)
	valueAttrRe = regexp.MustCompile(`(?i)\bvalue\s*=\s*"([^"]*)"`)
)

// Login 复刻 I4399GameSDKAPI.login
// 需要先调用 registerDevice
func (c *I4399Client) Login(ctx context.Context, username, password string) (*Cookie4399, error) {
	if c.Session == nil {
		return nil, errors.New("session 尚未初始化, 请先调用 registerDevice")
	}
	return c.login(ctx, username, password, false)
}

func (c *I4399Client) login(ctx context.Context, username, password string, retried bool) (*Cookie4399, error) {
	// 复刻 Kotlin: cookies(I4399_API_URL).count() > 2
	preFullSession := len(c.Client.Jar.Cookies(mustURL(i4399Base+"/"))) > 2

	forms1 := cloneMap(c.Session)
	forms1["auth_action"] = "ORILOGIN"

	html2, err := c.doFormText(ctx, http.MethodPost, i4399Base+"/oauth2/authorize.do?channel=&sdk=op", toValues(forms1))
	if err != nil {
		return nil, err
	}
	forms2 := decodeForms(html2, defaultBefore)

	// 验证码处理
	if captchaID, ok := forms2["captcha_id"]; ok {
		if c.OnCaptcha == nil {
			return nil, errors.New("需要验证码, 但未设置 OnCaptcha 处理器")
		}
		img, err := c.doGetBytes(ctx, i4399Base+"/ptlogin/captcha.do?captchaId="+captchaID)
		if err != nil {
			return nil, err
		}
		code, err := c.OnCaptcha(img)
		if err != nil {
			return nil, err
		}
		forms2["captcha_id"] = captchaID
		forms2["captcha"] = code
	}

	forms2["username"] = username
	forms2["password"] = password

	resp, err := c.doFormResp(ctx, http.MethodPost, i4399Base+"/oauth2/loginAndAuthorize.do?channel=&sdk=op", toValues(forms2))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var text string
	switch resp.StatusCode {
	case http.StatusAccepted: // 202
		b, _ := io.ReadAll(resp.Body)
		return nil, errors.New(string(b))
	case http.StatusOK: // 200
		b, _ := io.ReadAll(resp.Body)
		text = string(b)

		if m := loginErrMsgRe.FindStringSubmatch(text); m != nil {
			return nil, errors.New(m[1])
		}
		if m := captchaErrMsgRe.FindStringSubmatch(text); m != nil {
			return nil, errors.New(m[1])
		}

		// 需要完成账号实名认证
		if strings.Contains(text, "/oauth2/setIdcardAndRealname.do") {
			if c.OnRealName == nil {
				return nil, errors.New("账号需要实名认证, 但未设置 OnRealName 处理器")
			}
			formsR := decodeForms(text, `<div class="input_wrap_id finput_wrap">`)
			name, card, err := c.OnRealName()
			if err != nil {
				return nil, err
			}
			formsR["realname"] = name
			formsR["idcard"] = card

			respR, err := c.doFormResp(ctx, http.MethodPost, i4399Base+"/oauth2/setIdcardAndRealname.do", toValues(formsR))
			if err != nil {
				return nil, err
			}
			defer respR.Body.Close()
			if respR.StatusCode != http.StatusFound {
				return nil, errors.New("实名信息设置失败")
			}
			loc := respR.Header.Get("Location")
			if loc == "" {
				return nil, errors.New("实名信息设置失败: 缺少 Location")
			}
			html, err := c.doGetText(ctx, resolveURL(i4399Base, loc))
			if err != nil {
				return nil, err
			}
			text = html
		} else {
			return nil, errors.New("未知错误")
		}
	case http.StatusFound: // 302
		loc := resp.Header.Get("Location")
		if loc == "" {
			return nil, errors.New("缺少重定向 Location")
		}
		html, err := c.doGetText(ctx, resolveURL(i4399Base, loc))
		if err != nil {
			return nil, err
		}
		text = html
	default:
		return nil, fmt.Errorf("状态异常: %d", resp.StatusCode)
	}

	// 解析最终结果 JSON: {"code":"100","result":{"uid":..,"state":".."},"message":".."}
	// 注意: code 有时是字符串 "100" 有时是数字 100, 需兼容两者
	var done struct {
		Code    flexInt `json:"code"`
		Message string  `json:"message"`
		Result  struct {
			UID   uint64 `json:"uid"`
			State string `json:"state"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(text), &done); err != nil {
		snippet := text
		if len(snippet) > 500 {
			snippet = snippet[:500]
		}
		return nil, fmt.Errorf("解析登录结果失败: %w; 响应: %s", err, snippet)
	}

	// 复刻 Kotlin: 本地无完整 cookie 时远程端无法重定向到实名认证界面, 需要再登录一次
	if done.Message == "该账号异常，无法登录" && !preFullSession && !retried {
		return c.login(ctx, username, password, true)
	}

	if int(done.Code) != 100 {
		return nil, errors.New(done.Message)
	}

	return newCookie4399(done.Result.UID, done.Result.State), nil
}

// flexInt 兼容 JSON 中字符串或数字两种形式的整数
type flexInt int64

func (f *flexInt) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*f = 0
		return nil
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return err
	}
	*f = flexInt(v)
	return nil
}

// flexUint64 兼容 JSON 中字符串或数字两种形式的无符号整数
type flexUint64 uint64

func (f *flexUint64) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*f = 0
		return nil
	}
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return err
	}
	*f = flexUint64(v)
	return nil
}

var (
	loginErrMsgRe   = regexp.MustCompile(`(?s)<p\s+class="warning_tips\s+global_ico"\s+id="login_err_msg">\s*(.*?)\s*</p>`)
	captchaErrMsgRe = regexp.MustCompile(`(?s)<p\s+class="ipt_tips ipt_tips_err global_ico"\s+id="captcha1_err_msg">\s*(.*?)\s*</p>`)
)

// ---- HTTP 辅助 ----

func mustURL(raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}
	return u
}

func resolveURL(base, ref string) string {
	u, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	b, err := url.Parse(base)
	if err != nil {
		return ref
	}
	return b.ResolveReference(u).String()
}

func cloneMap(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func toValues(m map[string]string) url.Values {
	v := url.Values{}
	for k, val := range m {
		v.Set(k, val)
	}
	return v
}

// fakeUA 复刻 Kotlin: "Mozilla/5.0 <16随机>/<16随机>"
func fakeUA() string {
	return "Mozilla/5.0 " + randomString(16, hexCharset) + "/" + randomString(16, hexCharset)
}

func (c *I4399Client) newRequest(ctx context.Context, method, rawURL string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", fakeUA())
	return req, nil
}

func (c *I4399Client) doForm(ctx context.Context, method, rawURL string, form url.Values) ([]byte, int, error) {
	resp, err := c.doFormResp(ctx, method, rawURL, form)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	return b, resp.StatusCode, err
}

func (c *I4399Client) doFormText(ctx context.Context, method, rawURL string, form url.Values) (string, error) {
	b, _, err := c.doForm(ctx, method, rawURL, form)
	return string(b), err
}

func (c *I4399Client) doFormResp(ctx context.Context, method, rawURL string, form url.Values) (*http.Response, error) {
	req, err := c.newRequest(ctx, method, rawURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return c.Client.Do(req)
}

func (c *I4399Client) doGetText(ctx context.Context, rawURL string) (string, error) {
	b, err := c.doGetBytes(ctx, rawURL)
	return string(b), err
}

func (c *I4399Client) doGetBytes(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := c.newRequest(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s 状态异常: %d", rawURL, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
