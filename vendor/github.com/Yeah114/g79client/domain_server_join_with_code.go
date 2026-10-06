package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// DomainJoinServerResponse 描述通过邀请码加入山头服的响应。
type DomainJoinServerResponse struct {
	Response
	Entities []map[string]any `json:"entities"`
}

// JoinDomainServerWithInviteCode 使用邀请码加入山头服。
func (c *Client) JoinDomainServerWithInviteCode(code string) (*DomainJoinServerResponse, error) {
	if strings.TrimSpace(code) == "" {
		return nil, fmt.Errorf("JoinDomainServerWithInviteCode: code 不能为空")
	}

	api := "/domain-server/join-server-with-invite-code"
	requestData := map[string]any{
		"code": code,
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

	var joinResp DomainJoinServerResponse
	if err := json.Unmarshal(respBody, &joinResp); err != nil {
		return nil, fmt.Errorf("解析山头服邀请码加入响应失败: %v, 响应内容: %s", err, string(respBody))
	}

	return &joinResp, nil
}
