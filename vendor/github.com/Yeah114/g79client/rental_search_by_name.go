package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type SearchRentalServerResponse struct {
	Response
	Entities []RentalServerEntity `json:"entities"`
}

// 按名称搜索租赁服
func (c *Client) SearchRentalServerByName(serverName string) (*SearchRentalServerResponse, error) {
	api := "/rental-server/query/search-by-name"

	requestData := map[string]interface{}{
		"server_name": serverName,
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

	var searchResp SearchRentalServerResponse
	if err := json.Unmarshal(respBody, &searchResp); err != nil {
		return nil, fmt.Errorf("解析搜索响应失败: %v", err)
	}
	return &searchResp, nil
}
