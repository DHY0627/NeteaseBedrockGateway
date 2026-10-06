package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// 网络游戏进入-获取服务器地址 响应实体
type PeGameGetServerAddressEntity struct {
	IP          string    `json:"ip"`
	Port        Uncertain `json:"port"`
	Host        string    `json:"host"`
	NeedUpgrade Uncertain `json:"need_upgrade"`
	ISPEnable   Uncertain `json:"isp_enable"`
	CUCCHost    string    `json:"cucc_host"`
	CTCCHost    string    `json:"ctcc_host"`
	CMCCHost    string    `json:"cmcc_host"`
	CUCCPort    Uncertain `json:"cucc_port"`
	CTCCPort    Uncertain `json:"ctcc_port"`
	CMCCPort    Uncertain `json:"cmcc_port"`
	BHasMall    Uncertain `json:"b_has_mall"`
}

// 网络游戏进入-获取服务器地址 响应结构
type PeGameGetServerAddressResponse struct {
	Response
	Entity PeGameGetServerAddressEntity `json:"entity"`
}

// 获取网络游戏进入的服务器地址
// 对应：POST /pe-game/query/get-server-address
// 参数：itemID（必填），ticket（可为空；nil 将以 JSON null 发送）
func (c *Client) GetPeGameServerAddress(itemID string) (*PeGameGetServerAddressResponse, error) {
	api := "/pe-game/query/get-server-address"

	requestData := map[string]interface{}{
		"item_id": itemID,
	}
	requestData["ticket"] = nil

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
	req.Header.Set("Accept-Encoding", "gzip")
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

	var getResp PeGameGetServerAddressResponse
	if err := json.Unmarshal(respBody, &getResp); err != nil {
		return nil, fmt.Errorf("解析网络游戏进入响应失败: %v, 响应内容: %s", err, string(respBody))
	}
	return &getResp, nil
}
