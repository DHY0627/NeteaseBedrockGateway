package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// 购买组件结构体
type PurchaseItemEntity struct {
	EntityID  Uncertain `json:"entity_id"`
	BuyType   Uncertain `json:"buy_type"`
	OrderInfo string    `json:"order_info"`
}

type PurchaseItemResponse struct {
	Response
	Entity PurchaseItemEntity `json:"entity"`
}

// 购买组件
func (c *Client) PurchaseItem(itemID string) (*PurchaseItemResponse, error) {
	api := "/pe-purchase-item/"

	requestData := map[string]interface{}{
		"item_id": itemID,
		"expertcomment_info": map[string]string{
			"expertcomment_id": "0",
			"video_url":        "0",
			"expert_id":        "0",
		},
		"buy_path":   "商城:" + itemID,
		"coupon_ids": []any{},
		"cdk_code":   "",
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
	req.Header.Set("User-Agent", "WPFLauncher/0.0.0.0")
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

	var purchaseResp PurchaseItemResponse
	if err := json.Unmarshal(respBody, &purchaseResp); err != nil {
		return nil, fmt.Errorf("解析购买组件失败: %v", err)
	}
	return &purchaseResp, nil
}
