package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// 在线大厅房间搜索条目（search-by-name-v2 返回的实体字段子集）
type OnlineLobbyRoomSearchEntity struct {
	RoomID               Uncertain   `json:"room_id"`
	EntityID             Uncertain   `json:"entity_id"`
	RoomName             string      `json:"room_name"`
	Slogan               string      `json:"slogan"`
	Password             Uncertain   `json:"password"`
	ResID                Uncertain   `json:"res_id"`
	ResName              string      `json:"res_name"`
	MaxCount             Uncertain   `json:"max_count"`
	CurNum               Uncertain   `json:"cur_num"`
	AllowSave            Uncertain   `json:"allow_save"`
	Visibility           Uncertain   `json:"visibility"`
	OwnerName            string      `json:"owner_name"`
	OwnerID              Uncertain   `json:"owner_id"`
	SaveID               string      `json:"save_id"`
	Version              string      `json:"version"`
	Tag                  string      `json:"tag"`
	MinLevel             Uncertain   `json:"min_level"`
	OrderID              Uncertain   `json:"order_id"`
	GameStatus           Uncertain   `json:"game_status"`
	SaveSize             Uncertain   `json:"save_size"`
	FIDs                 []Uncertain `json:"fids"`
	LobbyManifestVersion string      `json:"lobby_manifest_version"`
	BehaviourUUID        string      `json:"behaviour_uuid"`
	PlayingUUID          string      `json:"playing_uuid"`
	TeamID               string      `json:"team_id"`
	ChatGroupID          string      `json:"chat_group_id"`
	ChatGroupReady       string      `json:"chat_group_ready"`
	MemberUIDs           []string    `json:"member_uids"`
}

// ID 返回房间唯一标识，兼容 room_id/entity_id 两种字段
func (e OnlineLobbyRoomSearchEntity) ID() string {
	if id := e.RoomID.String(); id != "" {
		return id
	}
	return e.EntityID.String()
}

// 在线大厅房间搜索响应
type OnlineLobbyRoomSearchResponse struct {
	Response
	Entities []OnlineLobbyRoomSearchEntity `json:"entities"`
	Total    Uncertain                     `json:"total"`
}

// 通过关键词搜索在线大厅房间（对应 /online-lobby-room/query/search-by-name-v2）
// keyword 通常传入“房间号”或部分房名。length/offset 可用于分页，version/res_id 可留空或根据需要填写。
func (c *Client) SearchOnlineLobbyRoomByKeyword(keyword string, length, offset int) (*OnlineLobbyRoomSearchResponse, error) {
	api := "/online-lobby-room/query/search-by-name-v2"

	requestData := map[string]interface{}{
		"length":  length,
		"version": GameVersion,
		"res_id":  "",
		"keyword": keyword,
		"offset":  offset,
	}

	jsonData, err := json.Marshal(requestData)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", c.ReleaseJSON.ApiGatewayUrl+api, strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("user-id", c.UserID)

	token := CalculateDynamicToken(api, string(jsonData), c.UserToken)
	req.Header.Set("user-token", token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, err
	}

	var searchResp OnlineLobbyRoomSearchResponse
	if err := json.Unmarshal(respBody, &searchResp); err != nil {
		return nil, fmt.Errorf("解析在线大厅搜索响应失败: %v, 响应内容: %s", err, string(respBody))
	}
	return &searchResp, nil
}

// 简要结果结构
type OnlineLobbyRoomBrief struct {
	RoomID    string
	RoomName  string
	OwnerName string
}

// 根据“房间号”或关键字搜索并返回简要信息列表
func (c *Client) FindOnlineLobbyRoomsByRoomNumber(roomNumber string) ([]OnlineLobbyRoomBrief, error) {
	resp, err := c.SearchOnlineLobbyRoomByKeyword(roomNumber, 20, 0)
	if err != nil {
		return nil, err
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("搜索失败(%d): %s", resp.Code, resp.Message)
	}
	results := make([]OnlineLobbyRoomBrief, 0, len(resp.Entities))
	for _, e := range resp.Entities {
		results = append(results, OnlineLobbyRoomBrief{
			RoomID:    e.ID(),
			RoomName:  e.RoomName,
			OwnerName: e.OwnerName,
		})
	}
	return results, nil
}
