package wplauncher

import (
	"encoding/json"
	"strings"
	"testing"
)

// 两种登录态都必须满足 Sauth 接口。
var (
	_ Sauth = (*Cookie4399)(nil)
	_ Sauth = (*SauthCookie)(nil)
)

const sampleSauth = `{"aim_info":{"aim":"127.0.0.1","country":"CN","tz":"+0800","tzid":""},"gameid":"x19","app_channel":"netease","login_channel":"netease","platform":"ad","sdk_version":"3.9.0","sdkuid":"u1","sessionid":"tok1","timestamp":null,"realname":{"realname_type":2}}`

// 解析必须逐字节保留字段顺序与原始类型（官方注释明确顺序敏感）。
func TestParseSauthCookieKeepsOrderAndTypes(t *testing.T) {
	c, err := ParseSauthCookie(sampleSauth)
	if err != nil {
		t.Fatalf("ParseSauthCookie: %v", err)
	}
	if got := c.JSON(); got != sampleSauth {
		t.Fatalf("解析后 JSON 与原始内容不一致:\n got %s\nwant %s", got, sampleSauth)
	}

	want := []string{"aim_info", "gameid", "app_channel", "login_channel", "platform", "sdk_version", "sdkuid", "sessionid", "timestamp", "realname"}
	if got := c.Keys(); !equalStrings(got, want) {
		t.Fatalf("字段顺序 = %v，应为 %v", got, want)
	}

	// 字符串字段返回内容；对象返回原始 JSON 文本；null 解析为字符串时得到空串
	if v, _ := c.Get("sdkuid"); v != "u1" {
		t.Errorf("sdkuid = %q", v)
	}
	if v, ok := c.Get("timestamp"); !ok || v != "" {
		t.Errorf("timestamp = %q(ok=%v)，null 应存在且解析为空串", v, ok)
	}
	if v, _ := c.Get("realname"); v != `{"realname_type":2}` {
		t.Errorf("realname = %q", v)
	}
	if v, _ := c.Get("aim_info"); v != `{"aim":"127.0.0.1","country":"CN","tz":"+0800","tzid":""}` {
		t.Errorf("aim_info = %q", v)
	}
}

// 支持 {"sauth_json":"..."} 包裹与 BOM / 首尾空白。
func TestParseSauthCookieUnwrapsWrapper(t *testing.T) {
	wrapped, err := json.Marshal(map[string]string{"sauth_json": sampleSauth})
	if err != nil {
		t.Fatal(err)
	}
	c, err := ParseSauthCookie("\ufeff \n" + string(wrapped) + " \n")
	if err != nil {
		t.Fatalf("ParseSauthCookie(包裹): %v", err)
	}
	if c.JSON() != sampleSauth {
		t.Fatalf("包裹解开后内容不对: %s", c.JSON())
	}

	// 内层也是空字符串时应当报错
	if _, err := ParseSauthCookie(`{"sauth_json":"   "}`); err == nil {
		t.Fatal("内层为空时应当报错")
	}
}

func TestParseSauthCookieErrors(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"空内容", "   \n\t", "sauth 内容为空"},
		{"不是对象", `"just a string"`, "不是 JSON 对象"},
		{"数组", `[1,2]`, "不是 JSON 对象"},
		{"非法 JSON", `{`, "解析 sauth 失败"},
		{"没有字段", `{}`, "解析 sauth 失败: 没有字段"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseSauthCookie(tc.in)
			if err == nil {
				t.Fatalf("应当报错")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("错误 = %q，应包含 %q", err.Error(), tc.want)
			}
		})
	}
}

func TestSauthCookieMutators(t *testing.T) {
	c, err := ParseSauthCookie(`{"a":"1","b":"2"}`)
	if err != nil {
		t.Fatal(err)
	}

	c.Set("a", "9")           // 覆盖不改位置
	c.Set("c", "3")           // 追加
	c.SetIfAbsent("a", "zzz") // 已存在 → 不动
	c.SetIfAbsent("d", "4")   // 不存在 → 追加
	if got, want := c.JSON(), `{"a":"9","b":"2","c":"3","d":"4"}`; got != want {
		t.Fatalf("JSON = %s，应为 %s", got, want)
	}

	c.Delete("b")
	if got, want := c.JSON(), `{"a":"9","c":"3","d":"4"}`; got != want {
		t.Fatalf("删除后 JSON = %s，应为 %s", got, want)
	}
	if c.Has("b") {
		t.Fatal("b 应已被删除")
	}
	if _, ok := c.Get("nope"); ok {
		t.Fatal("不存在的字段不应有值")
	}
	if c.String() != c.JSON() {
		t.Fatal("String() 应等价于 JSON()")
	}
}

// Clone 必须深拷贝：改副本不影响原对象。
func TestSauthCookieCloneIsDeep(t *testing.T) {
	orig, err := ParseSauthCookie(sampleSauth)
	if err != nil {
		t.Fatal(err)
	}
	clone := orig.Clone()
	clone.Set("sdkuid", "hacked")
	clone.Delete("gameid")

	if orig.JSON() != sampleSauth {
		t.Fatalf("原对象被副本修改影响: %s", orig.JSON())
	}
	if !clone.Has("sdkuid") || clone.Has("gameid") {
		t.Fatalf("副本未按预期修改: %s", clone.JSON())
	}
}

// SauthJSON 只在 platform 为 ad 时改成 pc，且不改动原对象。
func TestSauthJSONPlatform(t *testing.T) {
	ad, err := ParseSauthCookie(sampleSauth)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ad.SauthJSON(), `"platform":"pc"`) {
		t.Fatalf("SauthJSON 应把 platform 换成 pc: %s", ad.SauthJSON())
	}
	if v, _ := ad.Get("platform"); v != "ad" {
		t.Fatalf("原对象不应被改动，platform = %q", v)
	}

	pc, err := ParseSauthCookie(`{"platform":"pc","sdkuid":"u"}`)
	if err != nil {
		t.Fatal(err)
	}
	if got := pc.SauthJSON(); got != pc.JSON() {
		t.Fatalf("platform=pc 时 SauthJSON 应与 JSON 相同: %s", got)
	}
}

// AuthJSON 是 {"sauth_json":"<内容>"} 包裹。
func TestSauthCookieAuthJSON(t *testing.T) {
	c, err := ParseSauthCookie(sampleSauth)
	if err != nil {
		t.Fatal(err)
	}
	var wrapper struct {
		SauthJSON string `json:"sauth_json"`
	}
	if err := json.Unmarshal([]byte(c.AuthJSON()), &wrapper); err != nil {
		t.Fatalf("AuthJSON 不是合法 JSON: %v", err)
	}
	if wrapper.SauthJSON != c.JSON() {
		t.Fatalf("AuthJSON 内层内容不对: %s", wrapper.SauthJSON)
	}
}

func TestSauthCookieUDIDFallback(t *testing.T) {
	c, err := ParseSauthCookie(`{"deviceid":"dev-1"}`)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.UDIDValue(); got != "dev-1" {
		t.Fatalf("udid 缺失时应回退 deviceid，实际 %q", got)
	}
	c.Set("udid", "ud-1")
	if got := c.UDIDValue(); got != "ud-1" {
		t.Fatalf("UDIDValue = %q", got)
	}

	empty, err := ParseSauthCookie(`{"gameid":"x19"}`)
	if err != nil {
		t.Fatal(err)
	}
	if got := empty.UDIDValue(); got != "" {
		t.Fatalf("都没有时应为空，实际 %q", got)
	}
}

func TestSauthCookieSessionAndSDKUid(t *testing.T) {
	c, err := ParseSauthCookie(`{"sdkuid":"u1","sessionid":"tok1"}`)
	if err != nil {
		t.Fatal(err)
	}
	if c.SDKUid() != "u1" || c.SessionID() != "tok1" {
		t.Fatalf("SDKUid/SessionID = %q/%q", c.SDKUid(), c.SessionID())
	}
}

// EnsureSauthDefaults 只补缺失字段，不覆盖已有值。
func TestEnsureSauthDefaults(t *testing.T) {
	c, err := ParseSauthCookie(`{"gameid":"x19","platform":"ad","sdk_version":"1.2.3","junk":"keep"}`)
	if err != nil {
		t.Fatal(err)
	}
	EnsureSauthDefaults(c, "")

	if v, _ := c.Get("sdk_version"); v != "1.2.3" {
		t.Errorf("已有 sdk_version 被覆盖: %q", v)
	}
	if v, _ := c.Get("platform"); v != "ad" {
		t.Errorf("已有 platform 被覆盖: %q", v)
	}
	if !c.Has("junk") {
		t.Error("无关字段被丢弃")
	}
	for _, k := range []string{"aim_info", "app_channel", "login_channel", "udid", "client_login_sn"} {
		if !c.Has(k) {
			t.Errorf("未补齐 %s: %s", k, c.JSON())
		}
	}
	if v, _ := c.Get("app_channel"); v != "netease" {
		t.Errorf("app_channel = %q", v)
	}
	if v, _ := c.Get("sdk_version_dummy"); v != "" {
		t.Errorf("不该写入的字段: %q", v)
	}
	// 未指定版本时用默认值
	c2, _ := ParseSauthCookie(`{"gameid":"x19"}`)
	EnsureSauthDefaults(c2, "")
	if v, _ := c2.Get("sdk_version"); v != DefaultNetEaseSDKVersion {
		t.Errorf("默认 sdk_version = %q", v)
	}
	// 指定版本时用指定值
	c3, _ := ParseSauthCookie(`{"gameid":"x19"}`)
	EnsureSauthDefaults(c3, "9.9.9")
	if v, _ := c3.Get("sdk_version"); v != "9.9.9" {
		t.Errorf("sdk_version = %q", v)
	}
	// nil 不应 panic
	EnsureSauthDefaults(nil, "")
}

// Cookie4399 通过 Sauth 接口暴露的字段与原结构一致。
func TestCookie4399ImplementsSauth(t *testing.T) {
	c := newCookie4399(123456789, "session-state")

	var sauth Sauth = c
	if got := sauth.RawJSON(); got != c.marshal() {
		t.Fatalf("RawJSON 应等于 marshal(): %s", got)
	}
	if sauth.RawJSON() != sauth.SauthJSON() {
		t.Logf("注意: 4399 登录态 platform 为 ad，RawJSON 与 SauthJSON 不同属预期")
	}
	if !strings.Contains(sauth.SauthJSON(), `"platform":"pc"`) {
		t.Fatalf("4399 登录态 SauthJSON 应为 pc: %s", sauth.SauthJSON())
	}
	if sauth.UDIDValue() != c.UDID || len(c.UDID) != 16 {
		t.Fatalf("UDIDValue = %q，UDID = %q", sauth.UDIDValue(), c.UDID)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(c.RawJSON()), &fields); err != nil {
		t.Fatalf("Cookie4399 JSON 解析失败: %v", err)
	}
	if got := string(fields["sdkuid"]); got != `"123456789"` {
		t.Fatalf("sdkuid = %s", got)
	}
	// AuthJSON 应能被 ParseSauthCookie 反解
	back, err := ParseSauthCookie(sauth.AuthJSON())
	if err != nil {
		t.Fatalf("AuthJSON 无法反解: %v", err)
	}
	if back.SDKUid() != "123456789" || back.SessionID() != "session-state" {
		t.Fatalf("反解结果不对: %s", back.JSON())
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
