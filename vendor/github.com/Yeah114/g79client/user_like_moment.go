package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// LikeMomentResponse 表示点赞动态接口的响应体。
type LikeMomentResponse struct {
	Response
	SummaryMD5 string        `json:"summary_md5"`
	Entity     SocialProfile `json:"entity"`
}

// LikeMoment 为指定动态点赞。
func (c *Client) LikeMoment(msgID string, pushID uint64) (*LikeMomentResponse, error) {
	if strings.TrimSpace(msgID) == "" {
		return nil, fmt.Errorf("LikeMoment: msgID 不能为空")
	}

	api := "/user-like-moment"

	requestData := map[string]any{
		"msg_id": msgID,
	}
	if pushID != 0 {
		requestData["push_id"] = pushID
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

	var result LikeMomentResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("解析点赞动态响应失败: %v, 响应内容: %s", err, string(respBody))
	}

	return &result, nil
}
