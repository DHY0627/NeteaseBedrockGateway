package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// ApplyFriendResponse 表示申请好友接口的响应体。
type ApplyFriendResponse struct {
	Response
}

// ApplyFriend 向指定玩家发送好友申请。
func (c *Client) ApplyFriend(fid uint64) (*ApplyFriendResponse, error) {
	api := "/user-apply-friend/"

	requestData := map[string]interface{}{
		"fid":      fid,
		"comment":  c.friendApplyComment(),
		"add_type": 5,
		"message":  "",
		"add_ui":   "好友搜索",
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

	var result ApplyFriendResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("解析好友申请响应失败: %v, 响应内容: %s", err, string(respBody))
	}

	return &result, nil
}

func (c *Client) friendApplyComment() string {
	if c.UserDetail != nil && c.UserDetail.Name != "" {
		return c.UserDetail.Name
	}

	detail, err := c.GetUserDetail()
	if err == nil && detail != nil {
		c.UserDetail = &detail.Entity
		if detail.Entity.Name != "" {
			return detail.Entity.Name
		}
	}

	return "g79client"
}
