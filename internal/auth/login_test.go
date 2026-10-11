package auth

import (
	"context"
	"strings"
	"testing"
)

// 登录方式解析：大小写不敏感、去掉空格、支持别名、空值按 4399。
func TestParseLoginMode(t *testing.T) {
	cases := []struct {
		in   string
		want LoginMode
	}{
		{"", Mode4399},
		{"   ", Mode4399},
		{"4399", Mode4399},
		{"NET", Mode4399},
		{"netease-email", ModeNetEaseEmail},
		{"NetEase_Email", ModeNetEaseEmail},
		{" Net Ease-Email ", ModeNetEaseEmail},
		{"email", ModeNetEaseEmail},
		{"phone", ModeNetEasePhone},
		{"Mobile", ModeNetEasePhone},
		{"netease_phone", ModeNetEasePhone},
		{"sms", ModeNetEaseSms},
		{"netease-sms", ModeNetEaseSms},
		{"sauth_json", ModeSauth},
		{"SAUTH", ModeSauth},
	}
	for _, tc := range cases {
		got, err := ParseLoginMode(tc.in)
		if err != nil {
			t.Errorf("ParseLoginMode(%q) 报错: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseLoginMode(%q) = %q，应为 %q", tc.in, got, tc.want)
		}
	}

	if _, err := ParseLoginMode("weibo"); err == nil {
		t.Fatal("未知登录方式应当报错")
	} else if !strings.Contains(err.Error(), "未知的登录方式") {
		t.Fatalf("错误提示不对: %v", err)
	}
}

func TestModeLabel(t *testing.T) {
	cases := map[LoginMode]string{
		Mode4399:         "4399 账号",
		ModeNetEaseEmail: "网易邮箱密码",
		ModeNetEasePhone: "网易手机号密码",
		ModeNetEaseSms:   "网易手机号短信",
		ModeSauth:        "SAuth 登录态",
		LoginMode("x"):   "4399 账号",
		"":               "4399 账号",
	}
	for mode, want := range cases {
		if got := ModeLabel(mode); got != want {
			t.Errorf("ModeLabel(%q) = %q，应为 %q", mode, got, want)
		}
	}
}

// 网易各种登录方式的前置校验（不发网络请求）。
func TestLoginNetEaseValidation(t *testing.T) {
	cases := []struct {
		name string
		cred Credential
		mode LoginMode
		want string
	}{
		{
			name: "邮箱缺少 @",
			cred: Credential{Account: "someone", Password: "pw"},
			mode: ModeNetEaseEmail,
			want: "需要完整邮箱地址",
		},
		{
			name: "手机号位数不对",
			cred: Credential{Account: "1380000", Password: "pw"},
			mode: ModeNetEasePhone,
			want: "需要 11 位手机号",
		},
		{
			name: "短信登录手机号位数不对",
			cred: Credential{Account: "1380000"},
			mode: ModeNetEaseSms,
			want: "需要 11 位手机号",
		},
		{
			name: "短信登录缺少验证码回调",
			cred: Credential{Account: "13800000000"},
			mode: ModeNetEaseSms,
			want: "需要验证码回调",
		},
		{
			name: "sauth 内容非法",
			cred: Credential{Sauth: "not-json"},
			mode: ModeSauth,
			want: "SAuth 登录态解析失败",
		},
		{
			name: "sauth 为空",
			cred: Credential{},
			mode: ModeSauth,
			want: "SAuth 登录态解析失败",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := loginNetEase(context.Background(), tc.cred, tc.mode)
			if err == nil {
				t.Fatalf("应当报错")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("错误 = %q，应包含 %q", err.Error(), tc.want)
			}
		})
	}
}

// 合法的 sauth 无需任何网络请求即可返回登录态。
func TestLoginNetEaseSauth(t *testing.T) {
	cookie, err := loginNetEase(context.Background(), Credential{
		Sauth: `{"gameid":"x19","sdkuid":"u1","sessionid":"tok1"}`,
	}, ModeSauth)
	if err != nil {
		t.Fatalf("loginNetEase(sauth): %v", err)
	}
	if !strings.Contains(cookie.RawJSON(), `"sdkuid":"u1"`) {
		t.Fatalf("sauth 内容不对: %s", cookie.RawJSON())
	}
}

// LoginWith 对未知登录方式直接报错，不做任何网络请求。
func TestLoginWithUnknownMode(t *testing.T) {
	_, _, err := LoginWith(context.Background(), Credential{Mode: LoginMode("weibo"), Account: "x"}, LoginOptions{})
	if err == nil || !strings.Contains(err.Error(), "未知的登录方式") {
		t.Fatalf("应当报未知登录方式，实际: %v", err)
	}
}

// LoginWith 走 sauth 时会先完成 sauth 解析，非法内容要如实报错。
func TestLoginWithSauthInvalid(t *testing.T) {
	_, _, err := LoginWith(context.Background(), Credential{Mode: ModeSauth, Sauth: "{}"}, LoginOptions{})
	if err == nil || !strings.Contains(err.Error(), "SAuth 登录态解析失败") {
		t.Fatalf("应当报 SAuth 解析失败，实际: %v", err)
	}
}

func TestAuthJSONNilAndValue(t *testing.T) {
	if got := AuthJSON(nil); got != "" {
		t.Fatalf("AuthJSON(nil) = %q", got)
	}
	cookie, err := loginNetEase(context.Background(), Credential{
		Sauth: `{"gameid":"x19","sdkuid":"u1","sessionid":"tok1"}`,
	}, ModeSauth)
	if err != nil {
		t.Fatal(err)
	}
	got := AuthJSON(cookie)
	if !strings.HasPrefix(got, `{"sauth_json":"`) || !strings.Contains(got, `sdkuid`) {
		t.Fatalf("AuthJSON 格式不对: %s", got)
	}
}
