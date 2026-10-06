package g79client

import (
	"net/http"
	"strconv"
	"time"
)

var EngineVersion = "3.4.5.273121"

func Refetch() {
	packList, _ := RefreshG79PackList()
	if neteasePack, ok := packList["netease"]; ok && neteasePack.Version != "" {
		EngineVersion = neteasePack.Version
	}
	_, _ = RefreshG79LatestVersion()
	_, _ = RefreshG79ReleaseJSON()
	_, _ = RefreshX19ReleaseJSON()
	_, _ = RefreshG79ChatServers()
	_, _ = RefreshG79LinkServers()
	_, _ = RefreshG79TransferServers()
}

func init() {
	Refetch()
	go func() {
		for {
			time.Sleep(time.Second * 60)
			Refetch()
		}
	}()
}

// Client 结构体
type Client struct {
	UserID             string
	UserToken          string
	Seed               string
	ReleaseJSON        G79ReleaseJSON
	X19ReleaseJSON     X19ReleaseJSON
	EngineVersion      string
	G79LatestVersion   string
	patchResourcesHash string
	UserDetail         *UserDetailEntity
	peUserLoginAfter   *PeUserLoginAfterResponse
	httpClient         *http.Client
	Cookie             string
}

// 创建新的客户端
func NewClient() (*Client, error) {
	c := &Client{
		EngineVersion: EngineVersion,
		httpClient:    &http.Client{},
	}
	var err error
	patchMeta, err := GetGlobalG79PatchMetadata()
	if err != nil {
		return nil, err
	}
	c.G79LatestVersion = patchMeta.Version
	c.patchResourcesHash = patchMeta.ResourcesHash
	ReleaseJSON, err := GetGlobalG79ReleaseJSON()
	if err != nil {
		return nil, err
	}
	c.ReleaseJSON = *ReleaseJSON
	X19ReleaseJSON, err := GetGlobalX19ReleaseJSON()
	if err != nil {
		return nil, err
	}
	c.X19ReleaseJSON = *X19ReleaseJSON
	return c, nil
}

// 设置用户凭证
func (c *Client) SetCredentials(userID, userToken string) {
	c.UserID = userID
	c.UserToken = userToken
}

// 获取用户ID的整数形式
func (c *Client) GetUserIDInt() (int64, error) {
	return strconv.ParseInt(c.UserID, 10, 64)
}
