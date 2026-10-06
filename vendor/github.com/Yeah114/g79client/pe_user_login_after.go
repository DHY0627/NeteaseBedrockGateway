package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// PeUserLoginAfterResponse 表示 /pe-user-login-after 接口的响应体。
type PeUserLoginAfterResponse struct {
	Response
	Entity UserSocialState `json:"entity"`
}

// GetPeUserLoginAfter 请求 /pe-user-login-after 接口并缓存响应。
func (c *Client) GetPeUserLoginAfter() (*PeUserLoginAfterResponse, error) {
	if c.peUserLoginAfter != nil {
		return c.peUserLoginAfter, nil
	}

	api := "/pe-user-login-after"
	payload := map[string]any{
		"client_type": "cocos",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", c.ReleaseJSON.ApiGatewayUrl+api, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("user-id", c.UserID)

	token := CalculateDynamicToken(api, string(body), c.UserToken)
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

	var result PeUserLoginAfterResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("解析 pe-user-login-after 响应失败: %v, 响应内容: %s", err, string(respBody))
	}

	c.peUserLoginAfter = &result
	return &result, nil
}
