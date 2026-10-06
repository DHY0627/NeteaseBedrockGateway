package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// DomainDeleteOtherServerResponse 描述删除他服记录的响应。
type DomainDeleteOtherServerResponse struct {
	Response
	Entity any `json:"entity"`
}

// DeleteOtherDomainServer 移除指定 sid 的他服记录。
func (c *Client) DeleteOtherDomainServer(sid string) (*DomainDeleteOtherServerResponse, error) {
	if strings.TrimSpace(sid) == "" {
		return nil, fmt.Errorf("DeleteOtherDomainServer: sid 不能为空")
	}

	api := "/domain-server/del-other-server"
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

	var deleteResp DomainDeleteOtherServerResponse
	if err := json.Unmarshal(respBody, &deleteResp); err != nil {
		return nil, fmt.Errorf("解析删除他服响应失败: %v, 响应内容: %s", err, string(respBody))
	}

	return &deleteResp, nil
}
