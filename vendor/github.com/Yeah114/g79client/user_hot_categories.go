package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// HotCategoriesResponse 表示热门话题列表响应。
type HotCategoriesResponse struct {
	Status int `json:"status"`
	Data   struct {
		Data []string `json:"data"`
	} `json:"data"`
}

// GetHotCategories 获取热门话题列表。
func (c *Client) GetHotCategories() (*HotCategoriesResponse, error) {
	after, err := c.GetPeUserLoginAfter()
	if err != nil {
		return nil, err
	}
	momentID := after.Entity.MomentID

	u := &url.URL{
		Scheme: "https",
		Host:   "x19-pyq.webcgi.163.com",
		Path:   "/message/apps/x19/client/hot_categories",
	}
	query := u.Query()
	query.Set("uid", momentID)
	u.RawQuery = query.Encode()

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := readResponseBody(resp)
	if err != nil {
		return nil, err
	}

	var result HotCategoriesResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析热门话题响应失败: %v, 响应内容: %s", err, string(body))
	}
	return &result, nil
}
