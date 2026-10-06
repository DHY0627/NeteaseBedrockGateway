package g79client

// SocialProfile 描述社交相关接口返回的统一用户档案。
type SocialProfile struct {
	ID                   Uncertain             `json:"id"`
	Nickname             string                `json:"nickname"`
	Signature            string                `json:"signature"`
	HeadImage            string                `json:"headImage"`
	FrameID              string                `json:"frame_id"`
	MomentID             string                `json:"moment_id"`
	HeadImageType        Uncertain             `json:"headImageType"`
	PublicFlag           Uncertain             `json:"public_flag"`
	PEGrowth             SocialProfileGrowth   `json:"pe_growth"`
	UserGameInfo         map[string]any        `json:"user_game_info"`
	FriendRecommend      Uncertain             `json:"friend_recommend"`
	FriendApply          Uncertain             `json:"friend_apply"`
	Tag                  []SocialProfileTag    `json:"tag"`
	IsDeveloper          bool                  `json:"is_developer"`
	Mark                 string                `json:"mark"`
	RechargeBenefitLevel Uncertain             `json:"recharge_benefit_level"`
	IsFriend             bool                  `json:"is_friend"`
	TodayLiked           []any                 `json:"today_liked"`
	StatisData           []SocialProfileStat   `json:"statis_data"`
	HomepageBG           Uncertain             `json:"homepage_bg"`
	VisitCardBG          Uncertain             `json:"visit_card_bg"`
	CityNo               Uncertain             `json:"city_no"`
	RechargeVIPLevel     Uncertain             `json:"recharge_vip_level"`
	RechargeVIPInfo      map[string]any        `json:"recharge_vip_info"`
	LBSInfo              *SocialProfileLBSInfo `json:"lbs_info"`
	OnlineStatus         Uncertain             `json:"online_status"`
	OnlineType           Uncertain             `json:"online_type"`
	ChatBubbleID         Uncertain             `json:"chat_bubble_id"`
	MsgBackgroundID      Uncertain             `json:"msg_background_id"`
	StaticURL            string                `json:"static_url"`
}

// SocialProfileGrowth 表示成长信息。
type SocialProfileGrowth struct {
	Lv         Uncertain `json:"lv"`
	Exp        Uncertain `json:"exp"`
	NeedExp    Uncertain `json:"need_exp"`
	Decorate   []any     `json:"decorate"`
	IsVIP      Uncertain `json:"is_vip"`
	IsVIPExpr  Uncertain `json:"is_vip_expr"`
	ChatBubble Uncertain `json:"chat_bubble_id"`
}

// SocialProfileTag 表示用户标签。
type SocialProfileTag struct {
	TagID   Uncertain `json:"tag_id"`
	TagName string    `json:"tag_name"`
	Likes   Uncertain `json:"likes"`
}

// SocialProfileStat 表示统计数据。
type SocialProfileStat struct {
	Type    Uncertain `json:"type"`
	Value   Uncertain `json:"value"`
	Visible Uncertain `json:"visible"`
}

// SocialProfileLBSInfo 表示地理位置。
type SocialProfileLBSInfo struct {
	Province string `json:"province"`
	City     string `json:"city"`
	Area     string `json:"area"`
}

// WechatBindReward 描述微信绑定奖励。
type WechatBindReward struct {
	URL string    `json:"url"`
	NB  Uncertain `json:"nb"`
	TP  Uncertain `json:"tp"`
}

// SocialWechatInfo 描述与微信相关的配置。
type SocialWechatInfo struct {
	BindReward  *WechatBindReward `json:"bind_reward"`
	WechatH5URL string            `json:"wechat_h5_url"`
}

// SocialSetting 描述社交隐私设置。
type SocialSetting struct {
	UnderageMode              bool      `json:"underage_mode"`
	BlockStrangers            bool      `json:"block_strangers"`
	BlockAllMessages          bool      `json:"block_all_messages"`
	BlockRepostedAndCommented bool      `json:"block_reposted_and_commented"`
	MessageVisibility         Uncertain `json:"message_visibility"`
}

// UserSocialState 汇总登录后接口与档案接口的公共字段。
type UserSocialState struct {
	HasMessage          bool              `json:"hasMessage"`
	RestCurrencyTime    Uncertain         `json:"rest_currency_time"`
	RemainReviseNameCnt Uncertain         `json:"remain_revise_name_cnt"`
	UsedName            string            `json:"used_name"`
	MomentID            string            `json:"moment_id"`
	PublicFlag          bool              `json:"public_flag"`
	CanPostVideo        bool              `json:"can_post_video"`
	NeedPhoneBind       bool              `json:"need_phone_bind"`
	IsPhoneBind         Uncertain         `json:"is_phone_bind"`
	UpdateTimeStamp     Uncertain         `json:"update_time_stamp"`
	IsNewReward         Uncertain         `json:"is_new_reward"`
	IsPhoneAccount      Uncertain         `json:"is_phone_account"`
	BanChatExpiredAt    Uncertain         `json:"ban_chat_expired_at"`
	ActivityBonus       []any             `json:"activity_bonus"`
	SingleGameEffect    []any             `json:"single_game_special_effect"`
	BanItemInfo         []any             `json:"ban_item_info"`
	OnlineStatus        Uncertain         `json:"online_status"`
	WechatWishItem      Uncertain         `json:"wechat_wish_item"`
	MyImageURL          string            `json:"my_image_url"`
	MailDelList         []any             `json:"mail_del_list"`
	FavoriteIIDStatus   Uncertain         `json:"favorite_iid_status"`
	IsBind              bool              `json:"is_bind"`
	WechatInfo          *SocialWechatInfo `json:"wechat_info"`
	WechatWishItemID    string            `json:"wechat_wish_item_id"`
	SocialSetting       *SocialSetting    `json:"social_setting"`
}
