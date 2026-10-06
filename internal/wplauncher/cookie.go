package wplauncher

import (
	"encoding/json"
	"strconv"
	"strings"
)

// Cookie4399 对应 Kotlin 端 WPLauncherCookie4399Com
// 即 4399 第三方 OAuth 登录成功后得到的登录态, 用于向 x19 换取 session
type Cookie4399 struct {
	AimInfo       string `json:"aim_info"`
	RealName      string `json:"realname"`
	GameID        string `json:"gameid"`
	AppChannel    string `json:"app_channel"`
	LoginChannel  string `json:"login_channel"`
	Platform      string `json:"platform"`
	SDKVersion    string `json:"sdk_version"`
	SDKUid        string `json:"sdkuid"`
	Session       string `json:"sessionid"`
	UDID          string `json:"udid"`
	ClientLoginSN string `json:"client_login_sn"`
	DeviceID      string `json:"deviceid"`
}

// newCookie4399 以 uid/state 创建默认字段的 4399 登录态
func newCookie4399(uid uint64, state string) *Cookie4399 {
	return &Cookie4399{
		AimInfo:       `{"aim":"127.0.0.1","country":"CN","tz":"+0800","tzid":""}`,
		RealName:      `{"realname_type":2}`,
		GameID:        "x19",
		AppChannel:    "4399com",
		LoginChannel:  "4399com",
		Platform:      "ad",
		SDKVersion:    "3.12.2",
		SDKUid:        strconv.FormatUint(uid, 10),
		Session:       state,
		UDID:          randomString(16, hexCharset),
		ClientLoginSN: randomString(16, hexCharset),
		DeviceID:      randomString(16, hexCharset),
	}
}

// JSON 序列化 (字段顺序与 Kotlin 声明顺序一致)
func (c *Cookie4399) marshal() string {
	b, err := json.Marshal(c)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// toSauthJSON 复刻 Kotlin: 把 platform 从 "ad" 替换为 "pc" 后的 JSON 字符串
func (c *Cookie4399) toSauthJSON() string {
	return strings.Replace(c.marshal(), `"platform":"ad"`, `"platform":"pc"`, 1)
}

// toWrappedCookie 复刻 Kotlin: {"sauth_json":"<cookie json>"}
func (c *Cookie4399) toWrappedCookie() []byte {
	w := struct {
		SauthJSON string `json:"sauth_json"`
	}{SauthJSON: c.marshal()}
	b, err := json.Marshal(&w)
	if err != nil {
		panic(err)
	}
	return b
}
