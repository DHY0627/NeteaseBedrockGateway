package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// 更新昵称
func (c *Client) UpdateNickname(nickname string) error {
	api := "/pe-nickname-setting/update"

	requestData := map[string]interface{}{
		"name": nickname,
	}

	jsonData, err := json.Marshal(requestData)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", c.ReleaseJSON.ApiGatewayUrl+api, strings.NewReader(string(jsonData)))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("user-id", c.UserID)

	token := CalculateDynamicToken(api, string(jsonData), c.UserToken)
	req.Header.Set("user-token", token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return err
	}

	var response Response
	if err := json.Unmarshal(respBody, &response); err != nil {
		return fmt.Errorf("解析响应失败: %v", err)
	}

	if response.Code != 0 {
		return fmt.Errorf("更新昵称失败: %s", response.Message)
	}

	if c.UserDetail != nil {
		c.UserDetail.Name = nickname
	}
	return nil
}
