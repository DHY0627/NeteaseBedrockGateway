package wplauncher

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Sauth 是 x19 认证链（uni_sauth → login-otp → authentication-otp）所需的登录态视图。
//
// 实现它的一共有两种来源：
//   - *Cookie4399：4399 第三方 OAuth 登录得到的登录态，字段固定；
//   - *SauthCookie：用户直接提供的一份 sauth_json（网易官方渠道 / 手动粘贴），
//     字段与顺序原样保留，不做任何增删。
//
// 之所以抽象成接口，是因为 SAuth 登录必须逐字节还原用户给出的 JSON：
// 网易侧校验收到的文本，字段顺序或类型变化都可能导致登录失败。
type Sauth interface {
	// RawJSON 返回登录态自身的 JSON（uni_sauth 上报用）。
	RawJSON() string
	// AuthJSON 返回 login-otp 需要的 {"sauth_json":"<登录态 JSON>"} 包裹形式。
	AuthJSON() string
	// SauthJSON 返回 authentication-otp 需要的 sauth_json 字符串。
	SauthJSON() string
	// UDIDValue 返回 environment.udid 应使用的值。
	UDIDValue() string
}

// RawJSON 实现 Sauth：返回字段顺序固定的登录态 JSON。
func (c *Cookie4399) RawJSON() string { return c.marshal() }

// UDIDValue 实现 Sauth。
func (c *Cookie4399) UDIDValue() string { return c.UDID }

// sauthField 是 sauth JSON 中的单个字段，Value 保存原始 JSON 值，
// 以便在字符串/数字/bool 之间原样透传（官方 SDK 上报的字段类型并不统一）。
type sauthField struct {
	Key   string
	Value json.RawMessage
}

// SauthCookie 表示一份任意来源的网易登录态（sauth_json）。
//
// 与 Cookie4399 不同，它显式保留字段的原始顺序与原始类型：
// 网易校验收到的 JSON 文本，字段顺序或类型变化都可能导致登录失败，
// 因此解析后再序列化必须逐字节还原用户/上游给出的内容。
type SauthCookie struct {
	fields []sauthField
}

// NewSauthCookie 创建一个空的 sauth。
func NewSauthCookie() *SauthCookie { return &SauthCookie{} }

// ParseSauthCookie 解析 sauth。既接受裸 JSON：
//
//	{"gameid":"x19","sdkuid":"...","sessionid":"...", ...}
//
// 也接受启动器常见的包裹形式：
//
//	{"sauth_json":"{\"gameid\":\"x19\", ...}"}
func ParseSauthCookie(raw string) (*SauthCookie, error) {
	// 去掉首部的空白与 BOM（顺序任意），避免从文件粘贴时多出的空行/BOM 影响后续判断
	s := strings.TrimLeft(raw, " \t\r\n\ufeff")
	if s == "" {
		return nil, errors.New("sauth 内容为空")
	}

	// 解开 {"sauth_json": "..."} 包裹
	if strings.HasPrefix(s, "{") && strings.Contains(s, `"sauth_json"`) {
		var wrapper struct {
			SauthJSON string `json:"sauth_json"`
		}
		if err := json.Unmarshal([]byte(s), &wrapper); err == nil {
			if inner := strings.TrimSpace(wrapper.SauthJSON); inner != "" {
				s = inner
			}
		}
	}

	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("解析 sauth 失败: %w", err)
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, errors.New("解析 sauth 失败: 不是 JSON 对象")
	}

	cookie := NewSauthCookie()
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("解析 sauth 失败: %w", err)
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, errors.New("解析 sauth 失败: 字段名不是字符串")
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, fmt.Errorf("解析 sauth 字段 %q 失败: %w", key, err)
		}
		cookie.setRaw(key, append(json.RawMessage(nil), raw...))
	}
	if len(cookie.fields) == 0 {
		return nil, errors.New("解析 sauth 失败: 没有字段")
	}
	// 只解析出 sauth_json 一个字段说明包裹内层是空的，这种登录态无法使用
	if len(cookie.fields) == 1 && cookie.fields[0].Key == "sauth_json" {
		return nil, errors.New("解析 sauth 失败: sauth_json 内容为空")
	}
	return cookie, nil
}

// setRaw 按原顺序写入字段，已存在则原地覆盖（不改变位置）。
func (c *SauthCookie) setRaw(key string, value json.RawMessage) {
	for i := range c.fields {
		if c.fields[i].Key == key {
			c.fields[i].Value = value
			return
		}
	}
	c.fields = append(c.fields, sauthField{Key: key, Value: value})
}

// setRawIfAbsent 仅在字段不存在时写入（用于补齐默认字段而不覆盖用户值）。
func (c *SauthCookie) setRawIfAbsent(key string, value json.RawMessage) {
	for i := range c.fields {
		if c.fields[i].Key == key {
			return
		}
	}
	c.fields = append(c.fields, sauthField{Key: key, Value: value})
}

// Set 写入一个字符串字段。
func (c *SauthCookie) Set(key, value string) {
	b, err := json.Marshal(value)
	if err != nil {
		return
	}
	c.setRaw(key, b)
}

// SetIfAbsent 仅在字段不存在时写入字符串字段。
func (c *SauthCookie) SetIfAbsent(key, value string) {
	b, err := json.Marshal(value)
	if err != nil {
		return
	}
	c.setRawIfAbsent(key, b)
}

// Get 读取字段。字符串字段返回其内容，其它类型返回原始 JSON 文本。
func (c *SauthCookie) Get(key string) (string, bool) {
	for _, f := range c.fields {
		if f.Key != key {
			continue
		}
		var s string
		if err := json.Unmarshal(f.Value, &s); err == nil {
			return s, true
		}
		return string(f.Value), true
	}
	return "", false
}

// Has 判断字段是否存在。
func (c *SauthCookie) Has(key string) bool {
	_, ok := c.Get(key)
	return ok
}

// Delete 删除字段。
func (c *SauthCookie) Delete(key string) {
	for i := range c.fields {
		if c.fields[i].Key == key {
			c.fields = append(c.fields[:i], c.fields[i+1:]...)
			return
		}
	}
}

// Keys 按顺序返回全部字段名。
func (c *SauthCookie) Keys() []string {
	out := make([]string, 0, len(c.fields))
	for _, f := range c.fields {
		out = append(out, f.Key)
	}
	return out
}

// Clone 深拷贝一份 sauth。
func (c *SauthCookie) Clone() *SauthCookie {
	out := &SauthCookie{fields: make([]sauthField, 0, len(c.fields))}
	for _, f := range c.fields {
		out.fields = append(out.fields, sauthField{
			Key:   f.Key,
			Value: append(json.RawMessage(nil), f.Value...),
		})
	}
	return out
}

// JSON 输出紧凑的 sauth JSON（字段顺序与解析时一致）。
func (c *SauthCookie) JSON() string {
	var b strings.Builder
	b.Grow(256)
	b.WriteByte('{')
	for i, f := range c.fields {
		if i > 0 {
			b.WriteByte(',')
		}
		key, err := json.Marshal(f.Key)
		if err != nil {
			continue
		}
		b.Write(key)
		b.WriteByte(':')
		b.Write(f.Value)
	}
	b.WriteByte('}')
	return b.String()
}

// String 实现 fmt.Stringer，等价于 JSON()。
func (c *SauthCookie) String() string { return c.JSON() }

// RawJSON 实现 Sauth。
func (c *SauthCookie) RawJSON() string { return c.JSON() }

// AuthJSON 实现 Sauth：输出 login-otp 需要的 {"sauth_json":"<cookie json>"}。
func (c *SauthCookie) AuthJSON() string {
	w := struct {
		SauthJSON string `json:"sauth_json"`
	}{SauthJSON: c.JSON()}
	b, err := json.Marshal(&w)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// SauthJSON 实现 Sauth：与 JSON() 相同，但当 platform 为 "ad"（安卓渠道）时替换为 "pc"。
func (c *SauthCookie) SauthJSON() string {
	if v, ok := c.Get("platform"); ok && v == "ad" {
		clone := c.Clone()
		clone.Set("platform", "pc")
		return clone.JSON()
	}
	return c.JSON()
}

// UDIDValue 实现 Sauth：udid 缺失时回退到 deviceid。
func (c *SauthCookie) UDIDValue() string {
	if v, ok := c.Get("udid"); ok && v != "" {
		return v
	}
	if v, ok := c.Get("deviceid"); ok && v != "" {
		return v
	}
	return ""
}

// SessionID 返回 sessionid（mpay 的 user.token）。
func (c *SauthCookie) SessionID() string {
	v, _ := c.Get("sessionid")
	return v
}

// SDKUid 返回 sdkuid（mpay 的 user.id）。
func (c *SauthCookie) SDKUid() string {
	v, _ := c.Get("sdkuid")
	return v
}

// EnsureSauthDefaults 为外部传入的 sauth 补齐官方客户端常见的默认字段（不覆盖已有值）。
func EnsureSauthDefaults(c *SauthCookie, sdkVersion string) {
	if c == nil {
		return
	}
	if sdkVersion == "" {
		sdkVersion = DefaultNetEaseSDKVersion
	}
	c.SetIfAbsent("aim_info", `{"aim":"127.0.0.1","country":"CN","tz":"+0800","tzid":""}`)
	c.SetIfAbsent("gameid", "x19")
	c.SetIfAbsent("app_channel", "netease")
	c.SetIfAbsent("login_channel", "netease")
	c.SetIfAbsent("platform", "pc")
	c.SetIfAbsent("sdk_version", sdkVersion)
	c.SetIfAbsent("udid", randomString(16, hexCharset))
	c.SetIfAbsent("client_login_sn", randomString(16, hexCharset))
}

// DefaultNetEaseSDKVersion 是网易官方渠道 sauth 默认写入的 sdk_version。
const DefaultNetEaseSDKVersion = "3.9.0"
