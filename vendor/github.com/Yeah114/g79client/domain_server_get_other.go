package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// DomainOtherServerEntity 表示他服列表中的单个服信息。
type DomainOtherServerEntity struct {
	Sid        string    `json:"sid"`
	CreateTime Uncertain `json:"create_time"`
	EnterTime  Uncertain `json:"enter_time"`
	Name       string    `json:"name"`
	ExpireTime Uncertain `json:"expire_time"`
	Status     Uncertain `json:"status"`
	UserName   string    `json:"user_name"`
	UserID     string    `json:"user_id"`
}

// DomainOtherServersResponse 描述获取他服列表的响应。
type DomainOtherServersResponse struct {
	Response
	Entities []DomainOtherServerEntity `json:"entities"`
}

// GetOtherDomainServers 拉取当前账号加入的他服列表。
func (c *Client) GetOtherDomainServers() (*DomainOtherServersResponse, error) {
	api := "/domain-server/get-other-servers"
	body := "{}"

	req, err := http.NewRequest("POST", c.ReleaseJSON.ApiGatewayUrl+api, strings.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("user-id", c.UserID)

	token := CalculateDynamicToken(api, body, c.UserToken)
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

	var listResp DomainOtherServersResponse
	if err := json.Unmarshal(respBody, &listResp); err != nil {
		return nil, fmt.Errorf("解析山头服他服列表失败: %v, 响应内容: %s", err, string(respBody))
	}

	return &listResp, nil
}
