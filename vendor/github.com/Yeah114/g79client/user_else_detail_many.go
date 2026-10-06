package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// 批量他人详情响应
type UserElseDetailManyResponse struct {
	Response
	Entities   []SocialProfile `json:"entities"`
	Details    string          `json:"details"`
	SummaryMD5 string          `json:"summary_md5"`
}

// 获取他人详情（批量）
func (c *Client) GetUserElseDetailMany(uids []string) (*UserElseDetailManyResponse, error) {
	api := "/user-else-detail-many/"

	requestData := map[string]interface{}{
		"is_need_recharge_benefit": 1,
		"uids":                     strings.Join(uids, ";"),
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

	var result UserElseDetailManyResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("解析他人详情(批量)失败: %v", err)
	}
	return &result, nil
}
