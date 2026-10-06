package wplauncher

import (
	"context"
	"errors"
	"net/http"
)

// Options 登录选项
type Options struct {
	// OnCaptcha 4399 遇到图形验证码时回调, 传入验证码图片, 返回识别结果
	OnCaptcha func(image []byte) (string, error)
	// OnRealName 4399 账号需要实名认证时回调, 返回 (姓名, 身份证号)
	OnRealName func() (name, idCard string, err error)
	// Config 4399 第三方游戏配置, 默认 ConfigNetEaseMC
	Config I4399Config
	// HTTPClient 自定义 HTTP 客户端 (可选)
	HTTPClient *http.Client
	// SkipUniSauth 跳过 uni_sauth 上报步骤
	SkipUniSauth bool
}

type Option func(*Options)

func WithCaptchaHandler(f func(image []byte) (string, error)) Option {
	return func(o *Options) { o.OnCaptcha = f }
}

func WithRealNameHandler(f func() (name, idCard string, err error)) Option {
	return func(o *Options) { o.OnRealName = f }
}

func WithConfig(cfg I4399Config) Option {
	return func(o *Options) { o.Config = cfg }
}

func WithHTTPClient(c *http.Client) Option {
	return func(o *Options) { o.HTTPClient = c }
}

func WithSkipUniSauth() Option {
	return func(o *Options) { o.SkipUniSauth = true }
}

// Login 完整登录链路:
// 4399 OAuth 登录 -> uni_sauth 上报 -> login-otp -> authentication-otp -> Session
func Login(ctx context.Context, username, password string, opts ...Option) (*Session, error) {
	opt := &Options{Config: ConfigNetEaseMC}
	for _, o := range opts {
		o(opt)
	}

	// 1. 4399 第三方 OAuth 登录
	i4399, err := NewI4399Client(opt.Config)
	if err != nil {
		return nil, err
	}
	if opt.HTTPClient != nil {
		// 保留用户提供的 transport/timeout, 但必须带上 cookie jar
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
	cookie, err := i4399.Login(ctx, username, password)
	if err != nil {
		return nil, err
	}

	// 2. x19 认证
	x19 := NewX19Client(opt.HTTPClient)
	if !opt.SkipUniSauth {
		if err := x19.UniCookie(ctx, cookie); err != nil {
			return nil, err
		}
	}

	otp, err := x19.LoginCookie(ctx, cookie)
	if err != nil {
		return nil, err
	}

	latest, err := FetchLatestVersion(ctx, x19.Client)
	if err != nil {
		return nil, err
	}

	entity, err := x19.Authentication(ctx, cookie, otp, latest.Version)
	if err != nil {
		return nil, err
	}

	session := newSession(entity, x19)
	if session.ID == 0 {
		return nil, errors.New("登录成功但 entity_id 为空")
	}
	return session, nil
}
