// Package auth 封装登录 → x19 认证 → g79 客户端就绪 的完整流程。
//
// 支持两种账号渠道：
//
//	4399 渠道 : 4399 OAuth 登录（wplauncher）→ Cookie4399
//	网易渠道 : 网易账号登录（internal/netease，mpay 账号中台）→ SauthCookie
//
// 两条渠道在拿到登录态之后完全一致：
// uni_sauth 上报 → login-otp → authentication-otp → x19 Session。
// 认证成功后将 entity_id / token 写入 g79 客户端（SetCredentials），
// 使后续 TanLobbyCreate（创建房间）可以直接使用。
package auth

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"NeteaseBedrockGateway/internal/netease"
	"NeteaseBedrockGateway/internal/wplauncher"

	g79 "github.com/Yeah114/g79client"
)

// LoginOptions 登录选项。
type LoginOptions struct {
	// OnCaptcha 4399 图形验证码回调，返回识别结果。
	OnCaptcha func(image []byte) (string, error)
	// OnRealName 账号需要实名认证时的回调。
	OnRealName func() (name, idCard string, err error)
}

// LoginMode 是账号的登录方式。
type LoginMode string

const (
	// Mode4399 4399 账号 + 密码（原有方式）。
	Mode4399 LoginMode = "4399"
	// ModeNetEaseEmail 网易邮箱 + 密码。
	ModeNetEaseEmail LoginMode = "netease-email"
	// ModeNetEasePhone 网易手机号 + 密码。
	ModeNetEasePhone LoginMode = "netease-phone"
	// ModeNetEaseSms 网易手机号 + 短信验证码。
	ModeNetEaseSms LoginMode = "netease-sms"
	// ModeSauth 直接使用一段已有的 sauth_json 登录态。
	ModeSauth LoginMode = "sauth"
)

// ModeAliases 把外部配置里可能出现的写法归一化成标准登录方式。
var ModeAliases = map[string]LoginMode{
	"":              Mode4399,
	"4399":          Mode4399,
	"net":           Mode4399,
	"netease-email": ModeNetEaseEmail,
	"netease_email": ModeNetEaseEmail,
	"email":         ModeNetEaseEmail,
	"wy-email":      ModeNetEaseEmail,
	"net-email":     ModeNetEaseEmail,
	"netease-phone": ModeNetEasePhone,
	"netease_phone": ModeNetEasePhone,
	"phone":         ModeNetEasePhone,
	"mobile":        ModeNetEasePhone,
	"net-phone":     ModeNetEasePhone,
	"netease-sms":   ModeNetEaseSms,
	"netease_sms":   ModeNetEaseSms,
	"sms":           ModeNetEaseSms,
	"net-sms":       ModeNetEaseSms,
	"sauth":         ModeSauth,
	"sauth_json":    ModeSauth,
	"net-sauth":     ModeSauth,
}

// ParseLoginMode 解析登录方式字符串（大小写不敏感，支持常见别名，空值按 4399 处理）。
func ParseLoginMode(s string) (LoginMode, error) {
	key := strings.ToLower(strings.TrimSpace(s))
	key = strings.ReplaceAll(key, " ", "")
	if m, ok := ModeAliases[key]; ok {
		return m, nil
	}
	return "", fmt.Errorf("未知的登录方式 %q（可选：4399 / netease-email / netease-phone / netease-sms / sauth）", s)
}

// ModeLabel 返回登录方式的中文名，用于日志与界面。
func ModeLabel(m LoginMode) string {
	switch m {
	case ModeNetEaseEmail:
		return "网易邮箱密码"
	case ModeNetEasePhone:
		return "网易手机号密码"
	case ModeNetEaseSms:
		return "网易手机号短信"
	case ModeSauth:
		return "SAuth 登录态"
	default:
		return "4399 账号"
	}
}

// Credential 是一次登录所需的全部凭据。
type Credential struct {
	// Mode 登录方式，空值按 4399 处理。
	Mode LoginMode
	// Account 4399 用户名 / 网易邮箱 / 网易手机号。
	Account string
	// Password 4399 密码 / 网易密码；短信与 SAuth 方式留空。
	Password string
	// Sauth 仅 Mode == ModeSauth 时使用：一份 sauth_json（裸 JSON 或 {"sauth_json":"..."}）。
	Sauth string
	// OnSmsCode 短信验证码回调（要求返回验证码；上行短信场景返回任意非空字符串）。
	OnSmsCode func(challenge *netease.SmsChallenge) (string, error)
	// OnNotice 登录过程中的提示回调（设备注册、上行短信指引等）。
	OnNotice func(message string)
	// DeviceCachePath 网易设备指纹缓存文件路径；留空使用默认路径。
	DeviceCachePath string
}

// Login 完成 4399 登录 + x19 认证，返回已就绪（含 UserToken/UserDetail）的 g79 客户端。
//
// 这是 4399 渠道的专用入口，返回值带具体的 Cookie4399，便于调用方打印 sdkuid。
func Login(ctx context.Context, username, password string, opts LoginOptions) (*g79.Client, *wplauncher.Cookie4399, error) {
	// 1. 4399 OAuth 登录，获取登录态 Cookie（auth_json 数据源）
	var wlOpts []wplauncher.Option
	if opts.OnCaptcha != nil {
		wlOpts = append(wlOpts, wplauncher.WithCaptchaHandler(opts.OnCaptcha))
	}
	if opts.OnRealName != nil {
		wlOpts = append(wlOpts, wplauncher.WithRealNameHandler(opts.OnRealName))
	}
	cookie, err := wplauncher.Login4399(ctx, username, password, wlOpts...)
	if err != nil {
		return nil, nil, fmt.Errorf("4399 登录失败: %w", err)
	}

	cli, err := connect(ctx, cookie)
	if err != nil {
		return nil, nil, err
	}
	return cli, cookie, nil
}

// LoginWith 按 Credential.Mode 选择登录方式，完成登录 + x19 认证。
//
// 支持的四种网易登录方式：
//   - ModeNetEaseEmail：网易邮箱 + 密码
//   - ModeNetEasePhone：网易手机号 + 密码
//   - ModeNetEaseSms  ：网易手机号 + 短信验证码（含风控要求的上行短信）
//   - ModeSauth       ：直接使用一段已有的 sauth_json
func LoginWith(ctx context.Context, cred Credential, opts LoginOptions) (*g79.Client, wplauncher.Sauth, error) {
	mode := cred.Mode
	if mode == "" {
		mode = Mode4399
	}

	var cookie wplauncher.Sauth
	var err error

	switch mode {
	case Mode4399:
		var wlOpts []wplauncher.Option
		if opts.OnCaptcha != nil {
			wlOpts = append(wlOpts, wplauncher.WithCaptchaHandler(opts.OnCaptcha))
		}
		if opts.OnRealName != nil {
			wlOpts = append(wlOpts, wplauncher.WithRealNameHandler(opts.OnRealName))
		}
		cookie, err = wplauncher.Login4399(ctx, cred.Account, cred.Password, wlOpts...)
		if err != nil {
			return nil, nil, fmt.Errorf("4399 登录失败: %w", err)
		}

	case ModeNetEaseEmail, ModeNetEasePhone, ModeNetEaseSms, ModeSauth:
		cookie, err = loginNetEase(ctx, cred, mode)
		if err != nil {
			return nil, nil, err
		}

	default:
		return nil, nil, fmt.Errorf("未知的登录方式 %q", mode)
	}

	cli, err := connect(ctx, cookie)
	if err != nil {
		return nil, nil, err
	}
	return cli, cookie, nil
}

// loginNetEase 走网易账号（mpay）登录，返回 sauth 登录态。
func loginNetEase(ctx context.Context, cred Credential, mode LoginMode) (wplauncher.Sauth, error) {
	// sauth 登录态不是账号密码，单独处理：不需要设备注册与网络请求
	if mode == ModeSauth {
		cookie, err := netease.LoginBySauth(cred.Sauth)
		if err != nil {
			return nil, fmt.Errorf("SAuth 登录态解析失败: %w", err)
		}
		return cookie, nil
	}

	opts := []netease.Option{}
	if cred.OnNotice != nil {
		opts = append(opts, netease.WithNoticeHandler(cred.OnNotice))
	}
	if cred.OnSmsCode != nil {
		opts = append(opts, netease.WithSmsCodeHandler(cred.OnSmsCode))
	}
	if cred.DeviceCachePath != "" {
		opts = append(opts, netease.WithDeviceCacheFile(cred.DeviceCachePath))
	}

	var (
		cookie wplauncher.Sauth
		err    error
	)
	switch mode {
	case ModeNetEaseEmail:
		if !strings.Contains(cred.Account, "@") {
			return nil, fmt.Errorf("网易邮箱登录需要完整邮箱地址，当前为 %q", cred.Account)
		}
		cookie, err = netease.LoginByEmail(ctx, cred.Account, cred.Password, opts...)
		if err != nil {
			return nil, fmt.Errorf("网易邮箱登录失败: %w", err)
		}
	case ModeNetEasePhone:
		if !netease.IsPhoneNumber(cred.Account) {
			return nil, fmt.Errorf("网易手机号登录需要 11 位手机号，当前为 %q", cred.Account)
		}
		cookie, err = netease.LoginByPhonePassword(ctx, cred.Account, cred.Password, opts...)
		if err != nil {
			return nil, fmt.Errorf("网易手机号登录失败: %w", err)
		}
	case ModeNetEaseSms:
		if !netease.IsPhoneNumber(cred.Account) {
			return nil, fmt.Errorf("网易短信登录需要 11 位手机号，当前为 %q", cred.Account)
		}
		if cred.OnSmsCode == nil {
			return nil, fmt.Errorf("网易短信登录需要验证码回调（控制台请在实例卡片输入验证码，命令行请用 -sms-code 或交互输入）")
		}
		cookie, err = netease.LoginByPhoneSms(ctx, cred.Account, opts...)
		if err != nil {
			return nil, fmt.Errorf("网易短信登录失败: %w", err)
		}
	}
	return cookie, nil
}

// connect 初始化 g79 客户端并完成 x19 认证，返回可直接开房的客户端。
func connect(ctx context.Context, cookie wplauncher.Sauth) (*g79.Client, error) {
	cli, err := g79.NewClient()
	if err != nil {
		return nil, fmt.Errorf("初始化 g79 客户端失败: %w", err)
	}
	if err := authenticateX19(ctx, cli, cookie); err != nil {
		return nil, fmt.Errorf("x19 认证失败: %w", err)
	}
	detail, err := cli.GetUserDetail()
	if err != nil {
		return nil, fmt.Errorf("获取用户信息失败: %w", err)
	}
	cli.UserDetail = &detail.Entity
	return cli, nil
}

// authenticateX19 复刻 wplauncher 的 x19 认证链，
// 与 FunAuth 内置的 X19AuthenticateWithCookie 不同，
// 这里使用服务器兼容的请求体格式（参考 4399X19Login 的 X19Client.Authentication）。
func authenticateX19(ctx context.Context, cli *g79.Client, cookie wplauncher.Sauth) error {
	x19 := wplauncher.NewX19Client(nil)

	// uni_sauth 上报（可选步骤，失败不致命）
	_ = x19.UniCookie(ctx, cookie)

	// login-otp：获取一次性 OTP
	otp, err := x19.LoginCookie(ctx, cookie)
	if err != nil {
		return err
	}

	// 获取启动器最新版本号（用于 authentication-otp 的 version 字段）
	latest, err := wplauncher.FetchLatestVersion(ctx, x19.Client)
	if err != nil {
		return fmt.Errorf("获取最新版本失败: %w", err)
	}

	// authentication-otp：OTP 换取正式会话（entity_id + token）
	entity, err := x19.Authentication(ctx, cookie, otp, latest.Version)
	if err != nil {
		return err
	}
	if entity.EntityID == 0 {
		return fmt.Errorf("authentication 返回的 entity_id 为空")
	}

	// 写入 g79 客户端凭据
	cli.SetCredentials(strconv.FormatUint(uint64(entity.EntityID), 10), entity.Token)
	return nil
}

// AuthJSON 返回可用于调试/存档的 auth_json（sauth_json 包装格式）。
func AuthJSON(cookie wplauncher.Sauth) string {
	if cookie == nil {
		return ""
	}
	return cookie.AuthJSON()
}
