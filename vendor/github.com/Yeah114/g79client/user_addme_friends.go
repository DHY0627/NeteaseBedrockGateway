package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// AddMeFriendEntity 表示申请列表中的单个申请人。
type AddMeFriendEntity struct {
	UID                  Uncertain      `json:"uid"`
	Nickname             string         `json:"nickname"`
	HeadImage            string         `json:"headImage"`
	FrameID              string         `json:"frame_id"`
	ApplyComment         string         `json:"apply_comment"`
	ApplyMessage         string         `json:"apply_message"`
	ApplyTime            Uncertain      `json:"apply_time"`
	PEGrowth             map[string]any `json:"pe_growth"`
	MomentID             string         `json:"moment_id"`
	PublicFlag           bool           `json:"public_flag"`
	RechargeVIPInfo      map[string]any `json:"recharge_vip_info"`
	RechargeVIPLevel     Uncertain      `json:"recharge_vip_level"`
	RechargeBenefitLevel Uncertain      `json:"recharge_benefit_level"`
}

// AddMeFriendsResponse 表示拉取好友申请列表的响应结构。
type AddMeFriendsResponse struct {
	Response
	SummaryMD5 string              `json:"summary_md5"`
	Entities   []AddMeFriendEntity `json:"entities"`
}

// GetAddMeFriends 获取向当前玩家发起好友申请的列表。
func (c *Client) GetAddMeFriends() (*AddMeFriendsResponse, error) {
	api := "/user-addme-friends/"

	req, err := http.NewRequest("POST", c.ReleaseJSON.ApiGatewayUrl+api, strings.NewReader("{}"))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("user-id", c.UserID)

	token := CalculateDynamicToken(api, "{}", c.UserToken)
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

	var result AddMeFriendsResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("解析好友申请列表响应失败: %v, 响应内容: %s", err, string(respBody))
	}

	return &result, nil
}
