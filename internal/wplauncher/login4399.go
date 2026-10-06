package wplauncher

import (
	"context"
	"net/http"
)

// Login4399 仅执行 4399 OAuth 登录，返回 4399 登录态 Cookie4399。
//
// 与 Login 不同，它不继续 x19 认证链（uni_sauth / login-otp / authentication-otp），
// 而是直接返回可构造 auth_json 的 Cookie4399，供 FunAuth 的 PE 认证使用。
func Login4399(ctx context.Context, username, password string, opts ...Option) (*Cookie4399, error) {
	opt := &Options{Config: ConfigNetEaseMC}
	for _, o := range opts {
		o(opt)
	}

	i4399, err := NewI4399Client(opt.Config)
	if err != nil {
		return nil, err
	}
	if opt.HTTPClient != nil {
		jar := opt.HTTPClient.Jar
		if jar == nil {
			if jar, err = newCookieJar(); err != nil {
				return nil, err
			}
		}
		i4399.Client = &http.Client{
			Transport:     opt.HTTPClient.Transport,
			Timeout:       opt.HTTPClient.Timeout,
			Jar:           jar,
			CheckRedirect: i4399.Client.CheckRedirect,
		}
	}
	i4399.OnCaptcha = opt.OnCaptcha
	i4399.OnRealName = opt.OnRealName

	if err := i4399.registerDevice(ctx, ""); err != nil {
		return nil, err
	}
	return i4399.Login(ctx, username, password)
}
