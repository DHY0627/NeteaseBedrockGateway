package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// 在线大厅房间成长信息
type OnlineLobbyRoomPeGrowth struct {
	Decorate []any     `json:"decorate"`
	Exp      Uncertain `json:"exp"`
	IsVIP    Uncertain `json:"is_vip"`
	Lv       Uncertain `json:"lv"`
	NeedExp  Uncertain `json:"need_exp"`
}

// 在线大厅房间详情实体
type OnlineLobbyRoomGetEntity struct {
	AllowSave            Uncertain               `json:"allow_save"`
	BehaviourUUID        string                  `json:"behaviour_uuid"`
	ChatGroupID          Uncertain               `json:"chat_group_id"`
	ChatGroupReady       Uncertain               `json:"chat_group_ready"`
	CurNum               Uncertain               `json:"cur_num"`
	EntityID             Uncertain               `json:"entity_id"`
	Fids                 []string                `json:"fids"`
	GameStatus           Uncertain               `json:"game_status"`
	LobbyManifestVersion string                  `json:"lobby_manifest_version"`
	MaxCount             Uncertain               `json:"max_count"`
	MinLevel             Uncertain               `json:"min_level"`
	OrderID              Uncertain               `json:"order_id"`
	OwnerID              Uncertain               `json:"owner_id"`
	Password             Uncertain               `json:"password"`
	PeGrowth             OnlineLobbyRoomPeGrowth `json:"pe_growth"`
	PermissionInfo       map[string]any          `json:"permission_info"`
	PlayingUUID          string                  `json:"playing_uuid"`
	PVP                  Uncertain               `json:"pvp"`
	ResID                Uncertain               `json:"res_id"`
	RoomName             string                  `json:"room_name"`
	SaveID               string                  `json:"save_id"`
	SaveSize             Uncertain               `json:"save_size"`
	Slogan               string                  `json:"slogan"`
	Tag                  Uncertain               `json:"tag"`
	TeamID               Uncertain               `json:"team_id"`
	Version              string                  `json:"version"`
	Visibility           Uncertain               `json:"visibility"`
}

type OnlineLobbyRoomGetResponse struct {
	Response
	Entity OnlineLobbyRoomGetEntity `json:"entity"`
}

// 获取在线大厅房间详情（未加密响应）
func (c *Client) GetOnlineLobbyRoom(roomID string) (*OnlineLobbyRoomGetResponse, error) {
	api := "/online-lobby-room/get"

	requestData := map[string]interface{}{
		"room_id": roomID,
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

	var getResp OnlineLobbyRoomGetResponse
	if err := json.Unmarshal(respBody, &getResp); err != nil {
		return nil, fmt.Errorf("解析在线大厅详情响应失败: %v, 响应内容: %s", err, string(respBody))
	}
	return &getResp, nil
}
