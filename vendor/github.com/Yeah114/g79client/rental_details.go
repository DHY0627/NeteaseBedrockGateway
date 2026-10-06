package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// 租赁服详情（仅保留需要的字段）
type RentalServerDetailsEntity struct {
	Capacity Uncertain `json:"capacity"`
}

type GetRentalServerDetailsResponse struct {
	Response
	Entity RentalServerDetailsEntity `json:"entity"`
}

// 获取租赁服详情（含容量）
func (c *Client) GetRentalServerDetails(serverID string) (*GetRentalServerDetailsResponse, error) {
	api := "/rental-server-details/get"

	requestData := map[string]interface{}{
		"server_id": serverID,
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

	var detailsResp GetRentalServerDetailsResponse
	if err := json.Unmarshal(respBody, &detailsResp); err != nil {
		return nil, fmt.Errorf("解析租赁服详情响应失败: %v", err)
	}
	return &detailsResp, nil
}
