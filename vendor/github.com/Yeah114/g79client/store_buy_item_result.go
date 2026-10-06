package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// BuyItemResultEntity 表示购买结果中的单个组件信息。
type BuyItemResultEntity struct {
	EntityID        string    `json:"entity_id"`
	ItemID          string    `json:"item_id"`
	UserID          string    `json:"user_id"`
	PurchaseTime    Uncertain `json:"purchase_time"`
	LastPlayTime    Uncertain `json:"last_play_time"`
	TotalPlayTime   Uncertain `json:"total_play_time"`
	ExpireTime      Uncertain `json:"expire_time"`
	IsExpired       bool      `json:"is_expired"`
	IsItemTimeLimit Uncertain `json:"is_item_time_limit"`
	ItemRemainTime  Uncertain `json:"item_remain_time"`
}

// BuyItemResultResponse 表示购买结果查询响应。
type BuyItemResultResponse struct {
	Response
	Entity BuyItemResultEntity `json:"entity"`
}

// GetBuyItemResult 查询指定订单的购买结果。
func (c *Client) GetBuyItemResult(orderID string, buyType int) (*BuyItemResultResponse, error) {
	api := "/buy-item-result"

	requestData := map[string]any{
		"orderid":  orderID,
		"buy_type": buyType,
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

	var resultResp BuyItemResultResponse
	if err := json.Unmarshal(respBody, &resultResp); err != nil {
		return nil, fmt.Errorf("解析购买结果响应失败: %v", err)
	}
	return &resultResp, nil
}
