package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// UserItemPurchaseEntity 表示 /user-item-purchase 接口返回的实体数据。
type UserItemPurchaseEntity struct {
	EntityID string    `json:"entity_id"`
	BuyType  Uncertain `json:"buy_type"`
}

// UserItemPurchaseResponse 描述 /user-item-purchase 的响应结构。
type UserItemPurchaseResponse struct {
	Response
	Entity UserItemPurchaseEntity `json:"entity"`
}

// UserItemPurchase 购买指定资源 ID 的组件。
func (c *Client) UserItemPurchase(itemID string) (*UserItemPurchaseResponse, error) {
	if itemID == "" {
		return nil, fmt.Errorf("itemID is empty")
	}

	api := "/user-item-purchase"
	requestData := map[string]any{
		"entity_id":       0,
		"item_id":         itemID,
		"item_level":      0,
		"user_id":         c.UserID,
		"purchase_time":   0,
		"last_play_time":  0,
		"total_play_time": 0,
		"receiver_id":     "",
		"buy_path":        "PC_H5_COMPONENT_DETAIL",
		"coupon_ids":      []any{},
		"diamond":         0,
		"activity_name":   "",
		"batch_count":     1,
	}

	jsonData, err := json.Marshal(requestData)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", c.X19ReleaseJSON.DCWebURL+api, strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
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

	var purchaseResp UserItemPurchaseResponse
	if err := json.Unmarshal(respBody, &purchaseResp); err != nil {
		return nil, fmt.Errorf("解析用户组件购买响应失败: %v, 原始响应: %s", err, string(respBody))
	}
	return &purchaseResp, nil
}
