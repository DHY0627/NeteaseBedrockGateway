package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// DomainServerDetailEntity 描述山头服详情响应中的主要字段。
type DomainServerDetailEntity struct {
	Sid                  string              `json:"sid"`
	WorldID              string              `json:"world_id"`
	ServerType           string              `json:"server_type"`
	UID                  Uncertain           `json:"uid"`
	Name                 string              `json:"name"`
	Desc                 string              `json:"desc"`
	BackupID             Uncertain           `json:"backup_id"`
	CreateTime           Uncertain           `json:"create_time"`
	ExpireTime           Uncertain           `json:"expire_time"`
	Capacity             Uncertain           `json:"capacity"`
	Status               Uncertain           `json:"status"`
	AutoRecharge         Uncertain           `json:"auto_recharge"`
	SendCloseTipMail     string              `json:"send_close_tip_mail"`
	NextAutoRechargeTime string              `json:"next_auto_recharge_time"`
	ActiveType           Uncertain           `json:"active_type"`
	GoodsID              string              `json:"goods_id"`
	LastStartTime        Uncertain           `json:"last_start_time"`
	ServerProperties     map[string]any      `json:"server_properties"`
	ActiveComponents     any                  `json:"active_components"`
	BackupName           string              `json:"backup_name"`
	Version              string              `json:"version"`
	BackupTimestamp      string              `json:"backup_ts"`
	ServerHost           string              `json:"server_host"`
	ServerPort           Uncertain           `json:"server_port"`
	CMCCServerHost       string              `json:"cmcc_server_host"`
	CTCCServerHost       string              `json:"ctcc_server_host"`
	CUCCServerHost       string              `json:"cucc_server_host"`
	CMCCServerPort       Uncertain           `json:"cmcc_server_port"`
	CTCCServerPort       Uncertain           `json:"ctcc_server_port"`
	CUCCServerPort       Uncertain           `json:"cucc_server_port"`
	Lifecycle            string              `json:"lifecycle"`
	State                string              `json:"state"`
	LastEnterTime        Uncertain           `json:"last_enter_time"`
	OnlineCount          Uncertain           `json:"online_count"`
}

// DomainServerDetailResponse 是山头服详情接口的响应体。
type DomainServerDetailResponse struct {
	Response
	Entity DomainServerDetailEntity `json:"entity"`
}

// GetDomainServerDetail 拉取山头服详情。
func (c *Client) GetDomainServerDetail(sid string) (*DomainServerDetailResponse, error) {
	if strings.TrimSpace(sid) == "" {
		return nil, fmt.Errorf("GetDomainServerDetail: sid 不能为空")
	}

	api := "/domain-server/get-server-detail"
	requestData := map[string]any{
		"sid": sid,
	}

	body, err := json.Marshal(requestData)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", c.ReleaseJSON.ApiGatewayUrl+api, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("user-id", c.UserID)

	token := CalculateDynamicToken(api, string(body), c.UserToken)
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

	var detailResp DomainServerDetailResponse
	if err := json.Unmarshal(respBody, &detailResp); err != nil {
		return nil, fmt.Errorf("解析山头服详情响应失败: %v, 响应内容: %s", err, string(respBody))
	}

	return &detailResp, nil
}
