package netease

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"NeteaseBedrockGateway/internal/wplauncher"
)

// mpay 相关常量, 全部取自官方启动器 WPFLauncher 的请求参数。
const (
	// mpayHost 网易 mpay (账号中台) 地址
	mpayHost = "https://service.mkey.163.com"
	// defaultProject mpay project 标识
	defaultProject = "x19"
	// defaultGameVersion 游戏版本号
	defaultGameVersion = "c1.25.0"
	// defaultSDKVersion 写入 sauth 的 sdk_version
	defaultSDKVersion = wplauncher.DefaultNetEaseSDKVersion

	mpayAppKey    = "EEkEEXLymcNjM42yLY3Bn6AO15aGy4yq"
	mpayCV        = "c4.2.0"
	mpaySV        = "10.0.22621"
	mpayUpdaterCV = "c1.0.0"
	mpayArch      = "win_x64"
	mpayDefaultUA = "WPFLauncher/0.0.0.0"

	// mpayScope 对应官方 opt_fields
	mpayScope = "nickname,avatar,realname_status,mobile_bind_status,mask_related_mobile,related_login_status"
)

// User 是 mpay 返回的账号信息 (对应官方 MpayUser)。
type User struct {
	ID              string      `json:"id"`
	Token           string      `json:"token"`
	Nickname        string      `json:"nickname"`
	ClientUsername  string      `json:"client_username"`
	DisplayUsername string      `json:"display_username"`
	ExtAccessToken  string      `json:"ext_access_token"`
	NeedMask        bool        `json:"need_mask"`
	IsChannelAdult  bool        `json:"is_channel_adult"`
	ExtendInfo      *ExtendInfo `json:"pc_ext_info"`
}

// ExtendInfo 对应官方 MpayExtendInfo。
type ExtendInfo struct {
	SrcJFGameID     string `json:"src_jf_game_id"`
	SrcAppChannel   string `json:"src_app_channel"`
	FromGameID      string `json:"from_game_id"`
	SrcUDID         string `json:"src_udid"`
	SrcSDKVersion   string `json:"src_sdk_version"`
	SrcPayChannel   string `json:"src_pay_channel"`
	SrcClientIP     string `json:"src_client_ip"`
	ExtraUniSDKData string `json:"extra_unisdk_data"`
}

// Error mpay 返回的业务错误。
type Error struct {
	// StatusCode HTTP 状态码
	StatusCode int
	// Code 业务错误码 (0 表示无)
	Code int
	// Reason 错误原因
	Reason string
	// VerifyURL 需要跳转验证时的地址 (通常在 Code == 1351 时出现)
	VerifyURL string
	// ReplySMS 需要上行短信时的指引 (Code == 1373)
	ReplySMS *ReplySMS
	// Raw 原始响应体
	Raw string
}

// ReplySMS 上行短信指引。
type ReplySMS struct {
	Content string `json:"content"`
	Number  string `json:"number"`
}

// Error 实现 error。
func (e *Error) Error() string {
	msg := e.Reason
	if msg == "" {
		// 服务端未返回可读原因时, 至少保留 HTTP 状态码与原始片段
		if e.Raw == "" {
			msg = fmt.Sprintf("HTTP %d", e.StatusCode)
		} else {
			msg = fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Raw)
		}
	}
	if e.Code != 0 {
		msg = fmt.Sprintf("[%d] %s", e.Code, msg)
	}
	if e.Code == 1351 && e.VerifyURL != "" {
		msg += " 验证地址: " + e.VerifyURL
	}
	return "mpay: " + msg
}

// IsUpstreamSMS 是否为「需要先上行短信」的错误 (网易风控会要求用户主动发送一条短信)。
func (e *Error) IsUpstreamSMS() bool { return e.Code == 1373 }

// Client 是网易 mpay 账号中台客户端。
//
// 它负责设备注册, 以及三种账号登录方式（邮箱 + 密码、手机号 + 密码、手机号 + 短信验证码）。
// 登录成功后返回 User, 再由 BuildSauthCookie 转成可提交给 x19 的登录态。
type Client struct {
	// Client 自定义 HTTP 客户端
	Client *http.Client
	// Project mpay project, 默认 "x19"
	Project string
	// GameVersion 游戏版本, 默认 "c1.25.0"
	GameVersion string
	// ProcessID 进程 id 参数, 默认 "1000"
	ProcessID string
	// Device 直接指定已注册设备 (nil 时按 Cache -> 注册 的顺序获取)
	Device *Device
	// Cache 设备缓存
	Cache DeviceCache
	// UserAgent 请求 UA
	UserAgent string

	// Host 服务地址, 默认 https://service.mkey.163.com（本地 mock / 自建代理时可覆盖）
	Host string

	// OnNotice 可选的调试回调, 上报设备注册、上行短信指引等关键步骤
	OnNotice func(message string)
}

// NewClient 创建 mpay 客户端。
func NewClient(client *http.Client) *Client {
	return &Client{
		Client:      client,
		Project:     defaultProject,
		GameVersion: defaultGameVersion,
		ProcessID:   "1000",
		UserAgent:   mpayDefaultUA,
		Cache:       NewMemoryDeviceCache(),
	}
}

func (c *Client) project() string {
	if c.Project == "" {
		return defaultProject
	}
	return c.Project
}

func (c *Client) gameVersion() string {
	if c.GameVersion == "" {
		return defaultGameVersion
	}
	return c.GameVersion
}

func (c *Client) processID() string {
	if c.ProcessID == "" {
		return "1000"
	}
	return c.ProcessID
}

func (c *Client) userAgent() string {
	if c.UserAgent == "" {
		return mpayDefaultUA
	}
	return c.UserAgent
}

// host 返回服务地址（默认走官方域名）。
func (c *Client) host() string {
	if c.Host == "" {
		return mpayHost
	}
	return strings.TrimRight(c.Host, "/")
}

func (c *Client) httpClient() *http.Client {
	if c.Client == nil {
		c.Client = &http.Client{Timeout: 30 * time.Second}
	}
	return c.Client
}

func (c *Client) notice(msg string) {
	if c.OnNotice != nil {
		c.OnNotice(msg)
	}
}

// baseParams 构造官方启动器通用的基础参数。
func (c *Client) baseParams() url.Values {
	v := url.Values{}
	v.Set("app_channel", "netease")
	v.Set("app_mode", "2")
	v.Set("app_type", "games")
	v.Set("arch", mpayArch)
	v.Set("cv", mpayCV)
	v.Set("mcount_app_key", mpayAppKey)
	v.Set("mcount_transaction_id", "0")
	v.Set("process_id", c.processID())
	v.Set("sv", mpaySV)
	v.Set("updater_cv", mpayUpdaterCV)
	v.Set("game_id", c.project())
	v.Set("gv", c.gameVersion())
	return v
}

func parseError(status int, body []byte) error {
	e := &Error{StatusCode: status, Raw: truncate(strings.TrimSpace(string(body)), 512)}
	var parsed struct {
		Code      int       `json:"code"`
		Reason    string    `json:"reason"`
		Message   string    `json:"message"`
		VerifyURL string    `json:"verify_url"`
		ReplySMS  *ReplySMS `json:"reply_sms"`
	}
	if err := json.Unmarshal(body, &parsed); err == nil {
		e.Code = parsed.Code
		e.Reason = parsed.Reason
		if e.Reason == "" {
			e.Reason = parsed.Message
		}
		e.VerifyURL = parsed.VerifyURL
		e.ReplySMS = parsed.ReplySMS
	}
	return e
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// postForm 以 application/x-www-form-urlencoded 提交表单。
func (c *Client) postForm(ctx context.Context, path string, values url.Values) ([]byte, error) {
	rawURL := c.host() + path
	body := values.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", c.userAgent())

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return b, parseError(resp.StatusCode, b)
	}
	return b, nil
}

// RegisterDevice 向 mpay 注册一个新的设备指纹。
func (c *Client) RegisterDevice(ctx context.Context) (*Device, error) {
	uniqueID := randomString(32, "0123456789abcdef")

	v := c.baseParams()
	v.Set("unique_id", uniqueID)
	v.Set("brand", "Microsoft")
	v.Set("device_model", "pc_mode")
	v.Set("device_name", "Desktop-"+randomString(6, hexCharset))
	v.Set("device_type", "Computer")
	v.Set("init_urs_device", "0")
	v.Set("mac", randomMAC())
	v.Set("resolution", "1920x1080")
	v.Set("system_name", "windows")
	v.Set("system_version", mpaySV)

	path := fmt.Sprintf("/mpay/games/%s/devices", c.project())
	b, err := c.postForm(ctx, path, v)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Device *struct {
			ID  string `json:"id"`
			Key string `json:"key"`
		} `json:"device"`
	}
	if err := json.Unmarshal(b, &resp); err != nil {
		return nil, fmt.Errorf("解析设备注册响应失败: %w (body=%s)", err, truncate(string(b), 256))
	}
	if resp.Device == nil || resp.Device.ID == "" {
		return nil, fmt.Errorf("设备注册失败: 响应中没有 device.id (body=%s)", truncate(string(b), 256))
	}

	return &Device{
		UniqueID:    uniqueID,
		ID:          resp.Device.ID,
		Key:         resp.Device.Key,
		Project:     c.project(),
		GameVersion: c.gameVersion(),
	}, nil
}

// EnsureDevice 获取可用设备: 优先使用已指定的设备, 其次读取缓存, 最后才注册新设备。
// 注册结果会自动写入 Cache；写缓存失败只提示不中断（例如目录只读时仍可登录）。
func (c *Client) EnsureDevice(ctx context.Context) (*Device, error) {
	if d := c.Device; d.usable(c.project()) {
		return d, nil
	}
	if c.Cache != nil {
		cached, err := c.Cache.Load()
		if err != nil {
			return nil, err
		}
		if cached.usable(c.project()) {
			c.Device = cached
			return cached, nil
		}
	}

	c.notice("正在向网易注册设备指纹 ...")
	device, err := c.RegisterDevice(ctx)
	if err != nil {
		return nil, err
	}
	c.Device = device
	c.notice("设备注册成功: " + device.ID)
	if c.Cache != nil {
		if err := c.Cache.Save(device); err != nil {
			c.notice("警告: 保存设备缓存失败（下次登录会重新注册设备）: " + err.Error())
		}
	}
	return device, nil
}

// encryptLoginParams 复刻官方 MpayLoginEncrypt.encryptLoginParams：
// {"username":..,"password":md5(password),"unique_id":..} 经 AES(设备密钥)/ECB/PKCS7 后输出 hex。
func encryptLoginParams(device *Device, email, password string) (string, error) {
	key, err := device.aesKey()
	if err != nil {
		return "", err
	}
	plain, err := json.Marshal(struct {
		Username string `json:"username"`
		Password string `json:"password"`
		UniqueID string `json:"unique_id"`
	}{
		Username: email,
		Password: md5hex(password),
		UniqueID: device.UniqueID,
	})
	if err != nil {
		return "", err
	}
	return aesECBPKCS7Hex(plain, key)
}

func decodeUser(b []byte) (*User, error) {
	var resp struct {
		User *User `json:"user"`
	}
	if err := json.Unmarshal(b, &resp); err != nil {
		return nil, fmt.Errorf("解析 mpay 登录响应失败: %w (body=%s)", err, truncate(string(b), 256))
	}
	if resp.User == nil || resp.User.ID == "" || resp.User.Token == "" {
		return nil, fmt.Errorf("mpay 登录失败: 响应中没有有效的 user (body=%s)", truncate(string(b), 256))
	}
	return resp.User, nil
}

// LoginByEmail 使用网易邮箱 + 密码登录。
func (c *Client) LoginByEmail(ctx context.Context, email, password string) (*User, error) {
	device, err := c.EnsureDevice(ctx)
	if err != nil {
		return nil, err
	}
	params, err := encryptLoginParams(device, email, password)
	if err != nil {
		return nil, err
	}

	v := c.baseParams()
	v.Set("opt_fields", mpayScope)
	v.Set("params", params)
	v.Set("un", base64.StdEncoding.EncodeToString([]byte(email)))

	path := fmt.Sprintf("/mpay/games/%s/devices/%s/users", c.project(), url.PathEscape(device.ID))
	b, err := c.postForm(ctx, path, v)
	if err != nil {
		return nil, err
	}
	return decodeUser(b)
}

// SmsChallenge 描述一次短信验证码请求的结果。
type SmsChallenge struct {
	// Phone 手机号
	Phone string
	// Upstream 为 true 时表示网易风控要求先由用户主动发送一条短信, 之后才能收到验证码
	Upstream bool
	// Content 上行短信内容 (Upstream 为 true 时有效)
	Content string
	// Number 上行短信接收号码 (Upstream 为 true 时有效)
	Number string
	// Hint 服务端提示文案
	Hint string
}

// RequestSms 请求发送短信验证码。
// 当网易要求上行短信时, 不返回错误, 而是返回 Upstream == true 的 SmsChallenge。
func (c *Client) RequestSms(ctx context.Context, phone string) (*SmsChallenge, error) {
	device, err := c.EnsureDevice(ctx)
	if err != nil {
		return nil, err
	}

	v := c.baseParams()
	v.Set("device_id", device.ID)
	v.Set("mobile", phone)

	b, err := c.postForm(ctx, "/mpay/api/users/login/mobile/get_sms", v)
	challenge := &SmsChallenge{Phone: phone}
	if err != nil {
		var mpayErr *Error
		if errors.As(err, &mpayErr) && mpayErr.IsUpstreamSMS() && mpayErr.ReplySMS != nil {
			challenge.Upstream = true
			challenge.Content = mpayErr.ReplySMS.Content
			challenge.Number = mpayErr.ReplySMS.Number
			challenge.Hint = mpayErr.Reason
			c.notice(fmt.Sprintf("网易要求先上行短信: 发送 %q 到 %s", challenge.Content, challenge.Number))
			return challenge, nil
		}
		return nil, err
	}

	var resp struct {
		GuideText string `json:"guide_text"`
	}
	if err := json.Unmarshal(b, &resp); err == nil {
		challenge.Hint = resp.GuideText
	}
	return challenge, nil
}

// VerifySms 提交短信验证码换取 ticket。
// upContent 用于上行短信场景, 普通下行验证码传空字符串即可。
func (c *Client) VerifySms(ctx context.Context, phone, code, upContent string) (string, error) {
	device, err := c.EnsureDevice(ctx)
	if err != nil {
		return "", err
	}

	v := c.baseParams()
	v.Set("device_id", device.ID)
	v.Set("mobile", phone)
	v.Set("smscode", code)
	v.Set("up_content", upContent)

	return c.ticketFrom(c.postForm(ctx, "/mpay/api/users/login/mobile/verify_sms", v))
}

// VerifyPassword 使用手机号 + 密码换取 ticket。
func (c *Client) VerifyPassword(ctx context.Context, phone, password string) (string, error) {
	device, err := c.EnsureDevice(ctx)
	if err != nil {
		return "", err
	}

	now := time.Now().UnixMilli()
	v := c.baseParams()
	v.Set("device_id", device.ID)
	v.Set("login_for", "1")
	v.Set("mcount_transaction_id", "3")
	v.Set("mobile", phone)
	v.Set("opt_fields", mpayScope)
	v.Set("password", md5hex(password))
	v.Set("up_content", "")
	v.Set("transid", fmt.Sprintf("%s_%d_100022997", device.UniqueID, now))
	v.Set("uni_transaction_id", fmt.Sprintf("%s_%d_100023001", device.UniqueID, now))
	v.Set("urs_udid", device.UniqueID)

	path := "/mpay/api/users/login/mobile/verify_pwd?un=" + url.QueryEscape(base64.StdEncoding.EncodeToString([]byte(phone)))
	return c.ticketFrom(c.postForm(ctx, path, v))
}

// ticketFrom 从响应中取出 ticket。
func (c *Client) ticketFrom(b []byte, err error) (string, error) {
	if err != nil {
		return "", err
	}
	var resp struct {
		Ticket    string `json:"ticket"`
		GuideText string `json:"guide_text"`
	}
	if err := json.Unmarshal(b, &resp); err != nil {
		return "", fmt.Errorf("解析 mpay ticket 响应失败: %w (body=%s)", err, truncate(string(b), 256))
	}
	if resp.Ticket == "" {
		hint := resp.GuideText
		if hint == "" {
			hint = truncate(string(b), 256)
		}
		return "", fmt.Errorf("mpay 未返回 ticket: %s", hint)
	}
	return resp.Ticket, nil
}

// FinishPhoneLogin 使用 ticket 完成手机号登录。
func (c *Client) FinishPhoneLogin(ctx context.Context, phone, ticket string) (*User, error) {
	device, err := c.EnsureDevice(ctx)
	if err != nil {
		return nil, err
	}

	v := c.baseParams()
	v.Set("device_id", device.ID)
	v.Set("opt_fields", mpayScope)
	v.Set("ticket", ticket)

	path := "/mpay/api/users/login/mobile/finish?un=" + url.QueryEscape(base64.StdEncoding.EncodeToString([]byte(phone)))
	b, err := c.postForm(ctx, path, v)
	if err != nil {
		return nil, err
	}
	return decodeUser(b)
}

// BuildSauthCookie 把 mpay 登录结果组装成可提交给 x19 的 sauth 登录态。
// 字段顺序与官方启动器 WPLauncherCookieMpay 一致。
func BuildSauthCookie(user *User, device *Device, sdkVersion string) *wplauncher.SauthCookie {
	if sdkVersion == "" {
		sdkVersion = defaultSDKVersion
	}
	deviceID := ""
	if device != nil {
		deviceID = device.ID
	}
	c := wplauncher.NewSauthCookie()
	c.Set("aim_info", `{"aim":"127.0.0.1","country":"CN","tz":"+0800","tzid":""}`)
	c.Set("realname", `{"realname_type":2}`)
	c.Set("gameid", "x19")
	c.Set("app_channel", "netease")
	c.Set("login_channel", "netease")
	c.Set("platform", "pc")
	c.Set("sdk_version", sdkVersion)
	c.Set("sdkuid", user.ID)
	c.Set("sessionid", user.Token)
	c.Set("timestamp", "null")
	c.Set("userid", "null")
	c.Set("udid", randomString(16, hexCharset))
	c.Set("deviceid", deviceID)
	c.Set("client_login_sn", randomString(16, hexCharset))
	return c
}
