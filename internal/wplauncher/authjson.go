package wplauncher

// 本文件为 NeteaseBedrockGateway 项目新增的导出辅助方法：
// 将 4399 登录态 Cookie 转换为 FunAuth / g79client 所需的 auth_json。

// SauthJSON 返回 Cookie4399 的 sauth_json 字符串
// （platform 由 "ad" 替换为 "pc"，与登录链路中 authentication-otp 使用的格式一致）。
func (c *Cookie4399) SauthJSON() string {
	return c.toSauthJSON()
}

// AuthJSON 返回可直接用于 g79client.G79AuthenticateWithCookie 的完整 auth_json：
//
//	{"sauth_json": "<sauth_json 字符串>"}
func (c *Cookie4399) AuthJSON() string {
	return string(c.toWrappedCookie())
}
