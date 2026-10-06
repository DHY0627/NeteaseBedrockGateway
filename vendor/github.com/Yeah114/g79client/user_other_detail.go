package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// OtherUserDetailResponse 表示查询他人档案的响应体。
type OtherUserDetailResponse struct {
	Response
	Details    string        `json:"details"`
	SummaryMD5 string        `json:"summary_md5"`
	Entity     SocialProfile `json:"entity"`
}

// GetOtherUserDetail 查询指定玩家的详细档案。
func (c *Client) GetOtherUserDetail(entityID string, needRechargeBenefit bool) (*OtherUserDetailResponse, error) {
	if strings.TrimSpace(entityID) == "" {
		return nil, fmt.Errorf("GetOtherUserDetail: entityID 不能为空")
	}

	api := "/user-detail/query/other"

	requestData := map[string]any{
		"entity_id": entityID,
	}
	if needRechargeBenefit {
		requestData["is_need_recharge_benefit"] = 1
	} else {
		requestData["is_need_recharge_benefit"] = 0
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

	var result OtherUserDetailResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("解析他人档案响应失败: %v, 响应内容: %s", err, string(respBody))
	}

	return &result, nil
}
