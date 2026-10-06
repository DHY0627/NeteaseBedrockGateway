package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// ReplyFriendResponse 表示好友回复接口的响应体。
type ReplyFriendResponse struct {
	Response
	Entity struct {
		ReplyMsg string `json:"reply_msg"`
	} `json:"entity"`
}

// ReplyFriend 同意或拒绝好友申请。
func (c *Client) ReplyFriend(fid uint64, accept bool) (*ReplyFriendResponse, error) {
	api := "/user-reply-friend/"

	requestData := map[string]interface{}{
		"fid":       fid,
		"accept":    accept,
		"accept_ui": "联系人-好友申请",
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

	var result ReplyFriendResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("解析好友回复响应失败: %v, 响应内容: %s", err, string(respBody))
	}

	return &result, nil
}
