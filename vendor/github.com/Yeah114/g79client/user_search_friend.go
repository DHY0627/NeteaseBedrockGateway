package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// 搜索用户条目（按昵称/邮箱）
type SearchUserEntity struct {
	UID          Uncertain      `json:"uid"`
	Nickname     string         `json:"nickname"`
	HeadImage    string         `json:"headImage"`
	FrameID      string         `json:"frame_id"`
	MomentID     string         `json:"moment_id"`
	PublicFlag   Uncertain      `json:"public_flag"`
	OnlineStatus Uncertain      `json:"online_status"`
	OnlinePCPE   Uncertain      `json:"online_pcpe"`
	OnlineType   Uncertain      `json:"online_type"`
	GameInfo     map[string]any `json:"game_info"`
	PEGrowth     map[string]any `json:"pe_growth"`
	VipInfo      map[string]any `json:"recharge_vip_info"`
	VipLevel     Uncertain      `json:"recharge_vip_level"`
}

// 搜索用户响应
type SearchUserByNameOrMailResponse struct {
	Response
	Entities []SearchUserEntity `json:"entities"`
}

// 通过名称或邮箱搜索用户
// searchType: 0-精确 1-模糊；limit: 返回数量
func (c *Client) SearchUserByNameOrMail(nameOrMail string, searchType, limit int) (*SearchUserByNameOrMailResponse, error) {
	api := "/user-search-friend/"

	requestData := map[string]any{
		"name_or_mail": nameOrMail,
		"search_type":  searchType,
		"limit":        limit,
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

	var result SearchUserByNameOrMailResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("解析用户搜索响应失败: %v, 响应内容: %s", err, string(respBody))
	}
	return &result, nil
}
