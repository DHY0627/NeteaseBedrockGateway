package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// 租赁服进入结构体
type RentalServerWorldEntity struct {
	McserverHost string    `json:"mcserver_host"`
	McserverPort Uncertain `json:"mcserver_port"`
}

type EnterRentalServerResponse struct {
	Response
	Entity RentalServerWorldEntity `json:"entity"`
}

// 进入租赁服世界
func (c *Client) EnterRentalServerWorld(serverID, serverPassword string) (*EnterRentalServerResponse, error) {
	api := "/rental-server-world-enter/get"

	requestData := map[string]interface{}{
		"server_id": serverID,
		"pwd":       serverPassword,
	}

	jsonData, err := json.Marshal(requestData)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", c.ReleaseJSON.WebServerUrl+api, strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("User-Agent", "WPFLauncher/0.0.0.0")
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

	var enterResp EnterRentalServerResponse
	if err := json.Unmarshal(respBody, &enterResp); err != nil {
		return nil, fmt.Errorf("解析进入租赁服响应失败: %v", err)
	}
	return &enterResp, nil
}
