package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// 用户设置列表结构体
type UserSettingListEntity struct {
	SkinData struct {
		ItemID string `json:"item_id"`
	} `json:"skin_data"`
	ScreenConfig map[string]struct {
		ItemID        string    `json:"item_id"`
		BehaviourUUID string    `json:"behaviour_uuid"`
		OutfitLevel   Uncertain `json:"outfit_level"`
	} `json:"screen_config"`
}

type GetUserSettingListResponse struct {
	Response
	Entity UserSettingListEntity `json:"entity"`
}

// 获取用户设置列表
func (c *Client) GetUserSettingList() (*GetUserSettingListResponse, error) {
	api := "/pe-get-user-setting-list"

	requestData := map[string]interface{}{
		"settings": []string{
			"skin_type",
			"skin_data",
			"persona_data",
			"screen_config",
			"outfit_type",
			"personal_open",
			"personal_ad_open",
			"personal_tags",
			"bag_item",
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

	var getResp GetUserSettingListResponse
	if err := json.Unmarshal(respBody, &getResp); err != nil {
		return nil, fmt.Errorf("解析获取用户设置列表失败: %v", err)
	}
	return &getResp, nil
}
