// Package wplauncher 是 4399 -> x19 (我的世界中国版启动器) 登录链路的 Go 实现。
//
// 完整复刻 Kotlin 版 WPLauncherHelper 的核心逻辑, 仅依赖 Go 标准库。
//
// 链路:
//
//	4399 账号密码
//	  -> OAuth 会话 (m.4399api.com / ptlogin.4399.com)
//	  -> 4399 登录态 Cookie (uid + sessionid)
//	  -> uni_sauth 上报
//	  -> login-otp / authentication-otp
//	  -> x19 Session (entity_id + token)
//	  -> 自动心跳 authentication/update 持续刷新 token
//
// 最简用法:
//
//	session, err := wplauncher.Login(ctx, "username", "password")
//	if err != nil { /* 处理错误 */ }
//	session.StartHeartbeat(10 * time.Minute) // 保持在线
//
// 详细 API 文档见 API.md。
package wplauncher
