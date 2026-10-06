package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type SetUserSettingListResponse struct {
	Response
}

// 设置用户设置列表
func (c *Client) SetUserSettingList(itemID string) (*SetUserSettingListResponse, error) {
	api := "/pe-set-user-setting-list"

	requestData := map[string]interface{}{
		"data": map[string]any{
			"skin_data": map[string]any{
				"item_id": itemID,
			},
		},
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

	var setResp SetUserSettingListResponse
	if err := json.Unmarshal(respBody, &setResp); err != nil {
		return nil, fmt.Errorf("解析设置用户设置列表失败: %v", err)
	}
	return &setResp, nil
}
