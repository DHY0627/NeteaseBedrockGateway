package g79client

import (
	"encoding/json"
	"fmt"
	"strings"
)

// PlayerLocation 保存玩家当前所在位置的信息
type PlayerLocation struct {
	// 在线大厅
	RoomID string // 当前房间ID（若在大厅）
	// 租赁服
	ServerNo string // 服务器号（展示名，如租赁服号）
	ServerID string // 服务器ID（唯一ID）
}

// FindPlayerByNickname 根据昵称尝试定位玩家当前所在的房间或租赁服信息
// 逻辑：
// 1) 使用精确搜索接口按昵称查询用户，解析返回的 game_info 提取 room_id/server_id。
// 2) 若未解析到，再兜底用在线大厅关键字搜索匹配房主名/房间名。
func (c *Client) FindPlayerByNickname(nickname string) (*PlayerLocation, error) {
	if nickname == "" {
		return nil, fmt.Errorf("昵称不能为空")
	}

	// 优先：按昵称精确搜索用户
	userResp, err := c.SearchUserByNameOrMail(nickname, 0, 20)
	if err != nil {
		return nil, fmt.Errorf("搜索用户失败: %v", err)
	}
	if userResp.Code == 0 && len(userResp.Entities) > 0 {
		gi := userResp.Entities[0].GameInfo
		loc := &PlayerLocation{}
		if len(gi) > 0 {
			gameType := fmt.Sprint(gi["game-type"])    // 例如 1000（LobbyGame）
			gameID := fmt.Sprint(gi["game-id"])        // 可能是 room_id（LobbyGame）
			gameInfoStr := fmt.Sprint(gi["game-info"]) // JSON 字符串
			if gameInfoStr != "" {
				var gameInfo map[string]any
				if err := json.Unmarshal([]byte(gameInfoStr), &gameInfo); err == nil {
					if v, ok := gameInfo["room_id"]; ok {
						loc.RoomID = fmt.Sprint(v)
					}
					if v, ok := gameInfo["server_id"]; ok {
						loc.ServerID = fmt.Sprint(v)
					}
				}
			}
			if loc.RoomID == "" && gameType == "1000" && gameID != "" {
				loc.RoomID = gameID
			}
			if loc.RoomID != "" || loc.ServerID != "" {
				return loc, nil
			}
		}
	}

	// 兜底：在线大厅关键词搜索（匹配房主名/房间名）
	searchResp, err := c.SearchOnlineLobbyRoomByKeyword(nickname, 20, 0)
	if err == nil && searchResp != nil && searchResp.Code == 0 {
		for _, e := range searchResp.Entities {
			if e.OwnerName == nickname || e.RoomName == nickname || (e.OwnerName != "" && containsFold(e.OwnerName, nickname)) {
				return &PlayerLocation{RoomID: e.ID()}, nil
			}
		}
	}

	return nil, fmt.Errorf("未解析到玩家所在位置")
}

// containsFold 大小写不敏感包含判断（简化实现，避免引入strings.EqualFold以外依赖）
func containsFold(s, sub string) bool {
	// 直接使用标准库更稳妥
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}
