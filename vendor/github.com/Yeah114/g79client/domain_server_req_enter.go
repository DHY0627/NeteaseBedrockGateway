package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// RequestEnterDomainServer 申请进入指定山头服。
func (c *Client) RequestEnterDomainServer(sid string) (*DomainServerDetailResponse, error) {
	if strings.TrimSpace(sid) == "" {
		return nil, fmt.Errorf("RequestEnterDomainServer: sid 不能为空")
	}

	api := "/domain-server/req-enter-domain-server"
	requestData := map[string]any{
		"sid": sid,
	}

	body, err := json.Marshal(requestData)
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

	var enterResp DomainServerDetailResponse
	if err := json.Unmarshal(respBody, &enterResp); err != nil {
		return nil, fmt.Errorf("解析进入山头服响应失败: %v, 响应内容: %s", err, string(respBody))
	}

	return &enterResp, nil
}
