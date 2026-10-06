package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// 获取组件下载地址结构体
type GetDownloadInfoEntity struct {
	EntityID Uncertain `json:"entity_id"`
	ResURL   string    `json:"res_url"`
}

type GetDownloadInfoResponse struct {
	Response
	Entity GetDownloadInfoEntity `json:"entity"`
}

// 获取组件下载地址
func (c *Client) GetDownloadInfo(itemID string) (*GetDownloadInfoResponse, error) {
	api := "/pe-download-item/get-download-info"

	requestData := map[string]interface{}{
		"item_id": itemID,
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

	var getResp GetDownloadInfoResponse
	if err := json.Unmarshal(respBody, &getResp); err != nil {
		return nil, fmt.Errorf("解析获取组件下载地址失败: %v", err)
	}
	return &getResp, nil
}
