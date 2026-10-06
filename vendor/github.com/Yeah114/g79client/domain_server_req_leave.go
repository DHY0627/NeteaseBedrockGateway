package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// DomainLeaveServerResponse 描述退出山头服的响应。
type DomainLeaveServerResponse struct {
	Response
	Entity any `json:"entity"`
}

// RequestLeaveDomainServer 申请退出指定山头服。
func (c *Client) RequestLeaveDomainServer(sid string) (*DomainLeaveServerResponse, error) {
	if strings.TrimSpace(sid) == "" {
		return nil, fmt.Errorf("RequestLeaveDomainServer: sid 不能为空")
	}

	api := "/domain-server/req-leave-domain-server"
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

	// 设置请求头
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("user-id", c.UserID)

	// 计算动态Token并设置
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

	var leaveResp DomainLeaveServerResponse
	if err := json.Unmarshal(respBody, &leaveResp); err != nil {
		return nil, fmt.Errorf("解析退出山头服响应失败: %v, 响应内容: %s", err, string(respBody))
	}

	return &leaveResp, nil
}
