package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// 在线大厅房间进入实体
type OnlineLobbyRoomEnterEntity struct {
	RoomID Uncertain `json:"room_id"`
}

// 在线大厅房间进入结构体
type OnlineLobbyRoomEnterResponse struct {
	Response
	Entity OnlineLobbyRoomEnterEntity `json:"entity"`
}

// 进入在线大厅房间（未加密响应）
func (c *Client) EnterOnlineLobbyRoom(roomID, password string) (*OnlineLobbyRoomEnterResponse, error) {
	api := "/online-lobby-room-enter/"

	requestData := map[string]interface{}{
		"password":               password,
		"room_id":                roomID,
		"lobby_manifest_version": "",
		"check_visibilily":       1,
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

	var enterResp OnlineLobbyRoomEnterResponse
	if err := json.Unmarshal(respBody, &enterResp); err != nil {
		return nil, fmt.Errorf("解析在线大厅进入响应失败: %v, 响应内容: %s", err, string(respBody))
	}
	return &enterResp, nil
}
