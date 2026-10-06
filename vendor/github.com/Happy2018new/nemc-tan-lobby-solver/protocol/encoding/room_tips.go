package encoding

// RoomTips 描述网易本地联机房间的提示信息。
// 注意：真实网易客户端在 ProtocolID 之后还会携带一个版本字符串
// （如 "1.21.120.0"），玩家加入时用它做"游戏版本"校验。
// 缺少该字段会导致玩家提示"游戏版本不同"。
type RoomTips struct {
	LevelID            string
	GameType           uint8
	ConstantTestString string // Always is `Test`
	Vioce              int16
	ProtocolID         uint8
	// VersionString 是房间的游戏版本字符串（如 "1.21.120.0"）。
	// 真实客户端创建房间时会带上，玩家加入时校验版本一致性。
	VersionString string
}
