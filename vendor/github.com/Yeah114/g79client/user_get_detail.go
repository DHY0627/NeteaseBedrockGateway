package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// UserDetailResponse 表示查询自身档案的响应。
type UserDetailResponse struct {
	Response
	SummaryMD5 string           `json:"summary_md5"`
	Entity     UserDetailEntity `json:"entity"`
}

// UserDetailEntity 描述玩家的完整档案信息。
type UserDetailEntity struct {
	UserSocialState

	// 基础资料
	EntityID       string    `json:"entity_id"`
	Account        string    `json:"account"`
	Gender         string    `json:"gender"`
	Name           string    `json:"name"`
	Signature      string    `json:"signature"`
	AvatarImageURL string    `json:"avatar_image_url"`
	HeadImage      string    `json:"head_image"`
	FrameID        string    `json:"frame_id"`
	Aid            string    `json:"aid"`
	HeadImageType  Uncertain `json:"headImageType"`
	InstructInfo   string    `json:"instruct_info"`

	// 数值信息
	Level               Uncertain `json:"level"`
	Score               Uncertain `json:"score"`
	SkinNumber          Uncertain `json:"skin_number"`
	CapeNumber          Uncertain `json:"cape_number"`
	NicknameFree        Uncertain `json:"nickname_free"`
	NicknameInit        Uncertain `json:"nickname_init"`
	RealnameStatus      Uncertain `json:"realname_status"`
	RegisterTime        Uncertain `json:"register_time"`
	LoginTime           Uncertain `json:"login_time"`
	LogoutTime          Uncertain `json:"logout_time"`
	HeadImageCD         Uncertain `json:"head_image_cd"`
	AccessGameFlag      Uncertain `json:"access_game_flag"`
	AntiAdditionStatus  Uncertain `json:"anti_addition_status"`
	SubscribeExpireTime Uncertain `json:"subscribe_expiration_time"`
	VIPXDSubscribed     Uncertain `json:"vipxd_subscribed_benefit"`
	RechargeVIPLevel    Uncertain `json:"recharge_vip_level"`

	// 状态布尔量
	IsVIP                bool `json:"is_vip"`
	IsExprVIP            bool `json:"is_expr_vip"`
	IsSubscribe          bool `json:"is_subscribe"`
	IsAntiAddiction      bool `json:"isAntiAddiction"`
	NeedRealnameAuth     bool `json:"need_realname_auth"`
	CanBuyVIPSpecial     bool `json:"can_buy_vip_special"`
	CanBuyFirstChargeVIP bool `json:"can_buy_first_charge_vip"`
	CanUseExtraWorkbench bool `json:"can_use_extra_workbench"`

	// 复合数据
	NewChatBubbleID []any             `json:"new_chat_bubble_id"`
	VIPRecoverInfo  []any             `json:"vip_recover_info"`
	UnlockInfo      map[string]any    `json:"unlock_info"`
	RechargeVIPInfo map[string]any    `json:"recharge_vip_info"`
	DataVisible     map[string]any    `json:"data_visible"`
	VIPInfo         UserDetailVIPInfo `json:"vip_info"`
	UserGuideInfo   UserGuideInfo     `json:"user_guide_info"`
}

// UserDetailVIPInfo 描述 VIP 信息。
type UserDetailVIPInfo struct {
	Status            string    `json:"status"`
	BeginAt           Uncertain `json:"begin_at"`
	ExpiredAt         Uncertain `json:"expired_at"`
	ExprExpiredAt     Uncertain `json:"expr_expired_at"`
	AccumulativeTotal Uncertain `json:"accumulative_total"`
}

// UserGuideInfo 描述新手指引相关配置。
type UserGuideInfo struct {
	InitItem         []string                `json:"init_item"`
	SearchTag        string                  `json:"search_tag"`
	HomeIntroVideo   string                  `json:"home_intro_video"`
	RCIntroVideo     string                  `json:"rc_intro_video"`
	SingleGameVideo  string                  `json:"single_game_video"`
	NetGameVideo     string                  `json:"net_game_video"`
	LobbyGameVideo   string                  `json:"lobby_game_video"`
	RentalGameVideo  string                  `json:"rental_game_video"`
	HistoryViewVideo string                  `json:"history_view_video"`
	ShopKeeperVideo  string                  `json:"shop_keeper_video"`
	TrialItem        UserGuideTrialItem      `json:"trial_item"`
	TriggerVideo     []UserGuideTriggerVideo `json:"trigger_video"`
	Template         Uncertain               `json:"template"`
}

// UserGuideTrialItem 描述试用道具。
type UserGuideTrialItem struct {
	Weapon string `json:"weapon"`
	Mount  string `json:"mount"`
	Skin   string `json:"skin"`
}

// UserGuideTriggerVideo 描述触发式指引视频。
type UserGuideTriggerVideo struct {
	CoverURL string `json:"cover_url"`
	URL      string `json:"url"`
	Name     string `json:"name"`
	Intro    string `json:"intro"`
	Trigger  string `json:"trigger"`
}

// 获取用户详情
func (c *Client) GetUserDetail() (*UserDetailResponse, error) {
	api := "/pe-user-detail/get"

	requestData := map[string]interface{}{}

	jsonData, err := json.Marshal(requestData)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", c.ReleaseJSON.CoreServerURL+api, strings.NewReader(string(jsonData)))
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

	var userDetailResp UserDetailResponse
	if err := json.Unmarshal(respBody, &userDetailResp); err != nil {
		return nil, fmt.Errorf("解析用户详情响应失败: %v, 响应内容: %s", err, string(respBody))
	}
	return &userDetailResp, nil
}
