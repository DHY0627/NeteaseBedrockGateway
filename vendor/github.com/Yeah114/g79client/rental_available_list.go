package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// 租赁服条目
type RentalServerEntity struct {
	EntityID    Uncertain `json:"entity_id"`    // 唯一ID
	Name        Uncertain `json:"name"`         // 租赁服号
	OwnerID     Uncertain `json:"owner_id"`     // 服主ID
	Visibility  Uncertain `json:"visibility"`   // 可见性
	Status      Uncertain `json:"status"`       // 状态
	Capacity    Uncertain `json:"capacity"`     // 容量
	McVersion   string    `json:"mc_version"`   // 版本
	PlayerCount Uncertain `json:"player_count"` // 最大人数
	LikeNum     Uncertain `json:"like_num"`     // 点赞数
	ServerType  string    `json:"server_type"`  // 类型
	Offset      Uncertain `json:"offset"`       // 偏移量
	HasPwd      Uncertain `json:"has_pwd"`      // 是否需要密码
	ImageURL    string    `json:"image_url"`    // 租赁服头像URL
	WorldID     string    `json:"world_id"`     // 世界ID
	MinLevel    Uncertain `json:"min_level"`    // 最小加入等级
	PVP         Uncertain `json:"pvp"`          // 是否允许PVP
	ServerName  string    `json:"server_name"`  // 名称
}

// 批量拉取可用租赁服响应
type GetAvailableRentalServersResponse struct {
	Response
	Entities []RentalServerEntity `json:"entities"`
	Total    Uncertain            `json:"total"` // 总数
}

const (
	SortTypePlayerCount   = iota // 在线人数
	SortTypeNewest               // 最新
	SortTypeMostPopular          // 最受欢迎
	SortTypeComprehensive        // 综合
)

const (
	OrderTypeDesc = iota // 降序
	OrderTypeAsc         // 升序
)

// 按排序批量拉取可用租赁服列表
// sortType: 0-综合 1-在线人数 2-最新 3-最受欢迎
// orderType: 0-降序 1-升序
// offset: 偏移量
func (c *Client) GetAvailableRentalServers(sortType int, orderType int, offset int) (*GetAvailableRentalServersResponse, error) {
	api := "/rental-server/query/available-by-sort-type"

	requestData := map[string]interface{}{
		"sort_type":  sortType,
		"order_type": orderType,
		"offset":     offset,
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

	var listResp GetAvailableRentalServersResponse
	if err := json.Unmarshal(respBody, &listResp); err != nil {
		return nil, fmt.Errorf("解析可用租赁服列表失败: %v", err)
	}
	return &listResp, nil
}
