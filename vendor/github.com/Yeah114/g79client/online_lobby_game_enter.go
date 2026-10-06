package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type OnlineLobbyGameEnterResponse struct {
	Response
	Entity OnlineLobbyGameEnterEntity `json:"entity"`
}

// 在线大厅游戏进入实体
type OnlineLobbyGameEnterEntity struct {
	ISPEnable        Uncertain `json:"isp_enable"`
	ServerHost       string    `json:"server_host"`
	ServerHostV6     string    `json:"server_host_v6"`
	ServerPort       Uncertain `json:"server_port"`
	CMCCServerHost   string    `json:"cmcc_server_host"`
	CMCCServerHostV6 string    `json:"cmcc_server_host_v6"`
	CMCCServerPort   Uncertain `json:"cmcc_server_port"`
	CTCCServerHost   string    `json:"ctcc_server_host"`
	CTCCServerHostV6 string    `json:"ctcc_server_host_v6"`
	CTCCServerPort   Uncertain `json:"ctcc_server_port"`
	CUCCServerHost   string    `json:"cucc_server_host"`
	CUCCServerHostV6 string    `json:"cucc_server_host_v6"`
	CUCCServerPort   Uncertain `json:"cucc_server_port"`
}

// 进入在线大厅游戏（未加密响应）
func (c *Client) OnlineLobbyGameEnter() (*OnlineLobbyGameEnterResponse, error) {
	api := "/online-lobby-game-enter"

	requestData := map[string]interface{}{}
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

	var enterResp OnlineLobbyGameEnterResponse
	if err := json.Unmarshal(respBody, &enterResp); err != nil {
		return nil, fmt.Errorf("解析在线大厅游戏进入响应失败: %v, 响应内容: %s", err, string(respBody))
	}
	return &enterResp, nil
}
