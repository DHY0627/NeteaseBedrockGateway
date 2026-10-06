// Package auth 封装 4399 登录 → x19 认证 → g79 客户端就绪 的完整流程。
//
// 认证链路参考 4399X19Login（wplauncher）：
// 4399 OAuth 登录 → uni_sauth → login-otp → authentication-otp → x19 Session。
// 认证成功后将 entity_id / token 写入 g79 客户端（SetCredentials），
// 使后续 TanLobbyCreate（创建房间）可以直接使用。
package auth

import (
	"context"
	"fmt"
	"strconv"

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

// Login 完成 4399 登录 + x19 认证，返回已就绪（含 UserToken/UserDetail）的 g79 客户端。
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

	// 2. 初始化 g79 客户端
	cli, err := g79.NewClient()
	if err != nil {
		return nil, nil, fmt.Errorf("初始化 g79 客户端失败: %w", err)
	}

	// 3. x19 认证：uni_sauth → login-otp → authentication-otp
	if err := authenticateX19(ctx, cli, cookie); err != nil {
		return nil, nil, fmt.Errorf("x19 认证失败: %w", err)
	}

	// 4. 拉取用户详情
	detail, err := cli.GetUserDetail()
	if err != nil {
		return nil, nil, fmt.Errorf("获取用户信息失败: %w", err)
	}
	cli.UserDetail = &detail.Entity

	return cli, cookie, nil
}

// authenticateX19 复刻 wplauncher 的 x19 认证链，
// 与 FunAuth 内置的 X19AuthenticateWithCookie 不同，
// 这里使用服务器兼容的请求体格式（参考 4399X19Login 的 X19Client.Authentication）。
func authenticateX19(ctx context.Context, cli *g79.Client, cookie *wplauncher.Cookie4399) error {
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
func AuthJSON(cookie *wplauncher.Cookie4399) string {
	if cookie == nil {
		return ""
	}
	return cookie.AuthJSON()
}
