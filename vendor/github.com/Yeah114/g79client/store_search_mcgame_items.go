package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// McGameItemEntity 表示商城中单个组件的详细信息。
type McGameItemEntity struct {
	IID             Uncertain       `json:"iid"`
	Name            string          `json:"name"`
	TBuy            Uncertain       `json:"tBuy"`
	TExpire         Uncertain       `json:"tExpire"`
	Brief           string          `json:"brief"`
	Info            string          `json:"info"`
	MTypeID         Uncertain       `json:"mtypeid"`
	IncludeMap      Uncertain       `json:"include_map"`
	BalanceGrade    Uncertain       `json:"balance_grade"`
	AvailableScope  Uncertain       `json:"available_scope"`
	RelIID          Uncertain       `json:"rel_iid"`
	STypeID         Uncertain       `json:"stypeid"`
	VIPOnly         bool            `json:"vip_only"`
	EffectMTypeID   Uncertain       `json:"effect_mtypeid"`
	EffectSTypeID   Uncertain       `json:"effect_stypeid"`
	BehaviourUUID   string          `json:"behaviour_uuid"`
	ResourceUUID    string          `json:"resource_uuid"`
	ResVersion      Uncertain       `json:"resversion"`
	MCVersion       string          `json:"mcversion"`
	Versions        []string        `json:"versions"`
	RefundInfo      map[string]any  `json:"refund_info"`
	RequireItemList []any           `json:"require_item_list"`
	ExtData         json.RawMessage `json:"ext_data"`
}

// SearchMcGameItemsResponse 表示商城组件搜索结果。
type SearchMcGameItemsResponse struct {
	Response
	Entities []McGameItemEntity `json:"entities"`
	Total    Uncertain          `json:"total"`
}

// SearchMcGameItems 按组件ID或实体ID搜索商城组件。
func (c *Client) SearchMcGameItems(itemIDs []string) (*SearchMcGameItemsResponse, error) {
	api := "/item-buy-list/query/search-mcgame-items"

	if itemIDs == nil {
		itemIDs = []string{}
	}
	entityIDs := itemIDs

	requestData := map[string]any{
		"item_ids":   itemIDs,
		"entity_ids": entityIDs,
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

	var searchResp SearchMcGameItemsResponse
	if err := json.Unmarshal(respBody, &searchResp); err != nil {
		return nil, fmt.Errorf("解析组件搜索响应失败: %v", err)
	}
	return &searchResp, nil
}
