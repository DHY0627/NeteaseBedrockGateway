package netease

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"regexp"
	"strings"
	"time"

	"NeteaseBedrockGateway/internal/wplauncher"
)

// Options 登录选项。
type Options struct {
	// OnSmsCode 短信验证码回调, 手机号短信登录时必须提供。
	//   - 普通短信验证码: challenge.Upstream == false, 返回用户收到的验证码即可;
	//   - 网易风控要求上行短信: challenge.Upstream == true,
	//     请提示用户向 challenge.Number 发送 challenge.Content, 发送完成后返回任意非空字符串,
	//     本包随后会带 up_content 轮询直到拿到 ticket。
	OnSmsCode func(challenge *SmsChallenge) (string, error)

	// HTTPClient 自定义 HTTP 客户端 (可选)
	HTTPClient *http.Client

	// Device 直接指定已注册设备, 跳过设备注册与缓存
	Device *Device

	// DeviceCache 设备缓存, 默认 <用户目录>/.neteasebedrockgateway/device.json
	DeviceCache DeviceCache

	// Project mpay project, 默认 "x19"
	Project string

	// GameVersion 游戏版本, 默认 "c1.25.0"
	GameVersion string

	// SDKVersion 写入 sauth 的 sdk_version, 默认 "3.9.0"
	SDKVersion string

	// SmsInterval 上行短信轮询间隔, 默认 5s
	SmsInterval time.Duration

	// SmsRetry 上行短信轮询次数, 默认 24 (约 2 分钟)
	SmsRetry int

	// Host mpay 服务地址 (默认 https://service.mkey.163.com, 仅供测试/mock 使用)
	Host string

	// OnNotice 调试回调, 会上报设备注册、上行短信等关键步骤
	OnNotice func(message string)
}

// Option 登录选项构造函数。
type Option func(*Options)

// WithSmsCodeHandler 设置短信验证码回调。
func WithSmsCodeHandler(f func(challenge *SmsChallenge) (string, error)) Option {
	return func(o *Options) { o.OnSmsCode = f }
}

// WithHTTPClient 设置自定义 HTTP 客户端。
func WithHTTPClient(c *http.Client) Option {
	return func(o *Options) { o.HTTPClient = c }
}

// WithDevice 直接指定已注册设备。
func WithDevice(d *Device) Option {
	return func(o *Options) { o.Device = d }
}

// WithDeviceCache 设置设备缓存实现。
func WithDeviceCache(c DeviceCache) Option {
	return func(o *Options) { o.DeviceCache = c }
}

// WithDeviceCacheFile 使用指定路径的文件设备缓存。
func WithDeviceCacheFile(path string) Option {
	return func(o *Options) { o.DeviceCache = NewFileDeviceCache(path) }
}

// WithProject 设置 mpay project。
func WithProject(project string) Option {
	return func(o *Options) { o.Project = project }
}

// WithGameVersion 设置游戏版本号 (gv 参数)。
func WithGameVersion(v string) Option {
	return func(o *Options) { o.GameVersion = v }
}

// WithSDKVersion 设置写入 sauth 的 sdk_version。
func WithSDKVersion(v string) Option {
	return func(o *Options) { o.SDKVersion = v }
}

// WithSmsPolling 设置上行短信的轮询间隔与次数。
func WithSmsPolling(interval time.Duration, retry int) Option {
	return func(o *Options) {
		if interval > 0 {
			o.SmsInterval = interval
		}
		if retry > 0 {
			o.SmsRetry = retry
		}
	}
}

// WithNoticeHandler 设置调试回调。
func WithNoticeHandler(f func(message string)) Option {
	return func(o *Options) { o.OnNotice = f }
}

// WithHost 设置 mpay 服务地址（仅供测试/mock）。
func WithHost(host string) Option {
	return func(o *Options) { o.Host = host }
}

func newOptions(opts []Option) *Options {
	o := &Options{
		Project:     defaultProject,
		GameVersion: defaultGameVersion,
		SDKVersion:  defaultSDKVersion,
		SmsInterval: 5 * time.Second,
		SmsRetry:    24,
	}
	for _, fn := range opts {
		if fn != nil {
			fn(o)
		}
	}
	if o.DeviceCache == nil {
		o.DeviceCache = NewFileDeviceCache(DefaultDeviceCachePath())
	}
	return o
}

func (o *Options) notice(msg string) {
	if o.OnNotice != nil {
		o.OnNotice(msg)
	}
}

// httpClient 返回带 cookie jar 的 HTTP 客户端 (网易链路依赖会话 cookie)。
func (o *Options) httpClient() (*http.Client, error) {
	if o.HTTPClient == nil {
		jar, err := cookiejar.New(nil)
		if err != nil {
			return nil, err
		}
		return &http.Client{Timeout: 30 * time.Second, Jar: jar}, nil
	}
	if o.HTTPClient.Jar != nil {
		return o.HTTPClient, nil
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &http.Client{
		Transport:     o.HTTPClient.Transport,
		Timeout:       o.HTTPClient.Timeout,
		Jar:           jar,
		CheckRedirect: o.HTTPClient.CheckRedirect,
	}, nil
}

func (o *Options) mpayClient(client *http.Client) *Client {
	c := NewClient(client)
	c.Project = o.Project
	c.GameVersion = o.GameVersion
	c.Device = o.Device
	c.Cache = o.DeviceCache
	c.OnNotice = o.OnNotice
	c.Host = o.Host
	return c
}

// LoginByEmail 使用网易邮箱 + 密码登录, 返回可提交给 x19 的 sauth 登录态。
func LoginByEmail(ctx context.Context, email, password string, opts ...Option) (wplauncher.Sauth, error) {
	opt := newOptions(opts)
	client, err := opt.httpClient()
	if err != nil {
		return nil, err
	}
	mpay := opt.mpayClient(client)

	user, err := mpay.LoginByEmail(ctx, strings.TrimSpace(email), password)
	if err != nil {
		return nil, err
	}
	return BuildSauthCookie(user, mpay.Device, opt.SDKVersion), nil
}

// LoginByPhonePassword 使用手机号 + 密码登录。
func LoginByPhonePassword(ctx context.Context, phone, password string, opts ...Option) (wplauncher.Sauth, error) {
	opt := newOptions(opts)
	client, err := opt.httpClient()
	if err != nil {
		return nil, err
	}
	mpay := opt.mpayClient(client)

	phone = strings.TrimSpace(phone)
	ticket, err := mpay.VerifyPassword(ctx, phone, password)
	if err != nil {
		return nil, err
	}
	user, err := mpay.FinishPhoneLogin(ctx, phone, ticket)
	if err != nil {
		return nil, err
	}
	return BuildSauthCookie(user, mpay.Device, opt.SDKVersion), nil
}

// LoginByPhoneSms 使用手机号 + 短信验证码登录。
// 需要通过 WithSmsCodeHandler 提供验证码回调。
func LoginByPhoneSms(ctx context.Context, phone string, opts ...Option) (wplauncher.Sauth, error) {
	opt := newOptions(opts)
	client, err := opt.httpClient()
	if err != nil {
		return nil, err
	}
	mpay := opt.mpayClient(client)

	ticket, err := opt.obtainTicket(ctx, mpay, strings.TrimSpace(phone))
	if err != nil {
		return nil, err
	}
	user, err := mpay.FinishPhoneLogin(ctx, strings.TrimSpace(phone), ticket)
	if err != nil {
		return nil, err
	}
	return BuildSauthCookie(user, mpay.Device, opt.SDKVersion), nil
}

// LoginBySauth 直接使用一段已有的 sauth 登录态（不需要账号密码）。
// 参数既可以是裸 sauth JSON, 也可以是 {"sauth_json":"..."} 包裹形式。
func LoginBySauth(raw string) (wplauncher.Sauth, error) {
	cookie, err := wplauncher.ParseSauthCookie(raw)
	if err != nil {
		return nil, err
	}
	wplauncher.EnsureSauthDefaults(cookie, defaultSDKVersion)
	return cookie, nil
}

// obtainTicket 走完「请求验证码 -> 校验」的流程并返回 ticket。
func (o *Options) obtainTicket(ctx context.Context, mpay *Client, phone string) (string, error) {
	challenge, err := mpay.RequestSms(ctx, phone)
	if err != nil {
		return "", err
	}
	if o.OnSmsCode == nil {
		if challenge.Upstream {
			return "", fmt.Errorf("网易要求先上行短信 (向 %s 发送 %q), 需要提供验证码回调", challenge.Number, challenge.Content)
		}
		return "", errors.New("手机号短信登录需要验证码回调")
	}

	answer, err := o.OnSmsCode(challenge)
	if err != nil {
		return "", err
	}

	// 普通下行验证码
	if !challenge.Upstream {
		answer = strings.TrimSpace(answer)
		if answer == "" {
			return "", errors.New("短信验证码为空")
		}
		return mpay.VerifySms(ctx, phone, answer, "")
	}

	// 上行短信: 用户已按指引发送短信, 带 up_content 轮询
	upContent := challenge.Content
	interval := o.SmsInterval
	if interval <= 0 {
		interval = 5 * time.Second
	}
	retry := o.SmsRetry
	if retry <= 0 {
		retry = 1
	}

	var lastErr error
	for i := 0; i < retry; i++ {
		if i > 0 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(interval):
			}
		}
		ticket, err := mpay.VerifySms(ctx, phone, "", upContent)
		if err == nil {
			return ticket, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("短信验证未完成")
	}
	return "", fmt.Errorf("上行短信验证超时: %w", lastErr)
}

// IsPhoneNumber 判断字符串是否为 11 位手机号。
func IsPhoneNumber(s string) bool { return phonePattern.MatchString(strings.TrimSpace(s)) }

var phonePattern = regexp.MustCompile(`^1\d{10}$`)
