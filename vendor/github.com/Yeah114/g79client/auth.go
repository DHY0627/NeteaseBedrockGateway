package g79client

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/Yeah114/g79client/utils"
	"github.com/google/uuid"
)

// 认证相关结构体
type LoginEntity struct {
	EntityID string `json:"entity_id"`
	Token    string `json:"token"`
	Seed     string `json:"seed"`
}

type LoginResponse struct {
	Response
	Entity LoginEntity `json:"entity"`
}

// {'HostNum': 200, 'ServerHostNum': 8000, 'TempServerStop': 0, 'CdnUrl': 'https://g79.gdl.netease.com/', 'H5VersionUrl': 'https://g79.update.netease.com/cdnversion/obt_h5version.json', 'SeadraUrl': 'https://pub-api.seadra.netease.com', 'HomeServerUrl': 'https://g79mclobthome.minecraft.cn', 'HomeServerGrayUrl': 'https://g79mclobthomegray.nie.netease.com:9443', 'WebServerUrl': 'https://g79mclobt.minecraft.cn', 'WebServerGrayUrl': 'https://g79mclobtgray.nie.netease.com:9443', 'CoreServerUrl': 'https://g79obtapigtcoregray.minecraft.cn', 'CoreServerGrayUrl': 'https://g79obtapigtcoregray.minecraft.cn', 'TransferServerUrl': 'https://g79.update.netease.com/transferserver_obt_new.list', 'TransferServerHttpUrl': 'https://g79transfernew.nie.netease.com', 'TransferServerNewHttpUrl': 'https://g79mcltransfer.minecraft.cn', 'MomentUrl': 'https://x19-pyq.webcgi.163.com/', 'ForumUrl': 'https://mcpel-web.16163.com', 'AuthServerUrl': 'https://g79authobt.minecraft.cn', 'ChatServerUrl': 'https://x19.update.netease.com/chatserver.list', 'PathNUrl': 'https://impression.update.netease.com/lighten/atlas_x19_hangzhou-{isp}.txt', 'PePathNUrl': 'https://impression.update.netease.com/lighten/atlas_g79_hangzhou-{isp}.txt', 'PathNIpv6Url': 'https://impression.update.netease.com/lighten/x19/cnv6.txt', 'PePathNIpv6Url': 'https://impression.update.netease.com/lighten/g79/cnv6.txt', 'LinkServerUrl': 'https://g79.update.netease.com/linkserver_obt.list', 'ApiGatewayUrl': 'https://g79apigatewayobt.minecraft.cn', 'ApiGatewayWeiXinUrl': 'https://g79apigatewayobtweixin.minecraft.cn', 'ApiGatewayGrayUrl': 'https://g79apigatewaygrayobt.nie.netease.com', 'communityHost': 'https://news-api.16163.com/app/g79/api', 'WelfareUrl': 'https://mc.163.com/pe/client/', 'DCWebUrl': 'https://x19apigatewayobt.nie.netease.com', 'RentalTransferUrl': 'https://mcrealms.update.netease.com/isp_map_production.json', 'MgbSdkUrl': 'https://mgbsdk.matrix.netease.com'}
type G79ReleaseJSON struct {
	CoreServerURL            string `json:"CoreServerUrl"`
	AuthServerURL            string `json:"AuthServerUrl"`
	WebServerUrl             string `json:"WebServerUrl"`
	ApiGatewayUrl            string `json:"ApiGatewayUrl"`
	TransferServerUrl        string `json:"TransferServerUrl"`
	TransferServerNewHttpUrl string `json:"TransferServerNewHttpUrl"`
	ChatServerURL            string `json:"ChatServerUrl"`
	LinkServerURL            string `json:"LinkServerUrl"`
}

type X19ReleaseJSON struct {
	CoreServerURL              string `json:"CoreServerUrl"`
	WebServerURL               string `json:"WebServerUrl"`
	WebServerGrayURL           string `json:"WebServerGrayUrl"`
	TransferServerURL          string `json:"TransferServerUrl"`
	TransferServerHTTPURL      string `json:"TransferServerHttpUrl"`
	PeTransferServerURL        string `json:"PeTransferServerUrl"`
	PeTransferServerHTTPURL    string `json:"PeTransferServerHttpUrl"`
	PeTransferServerNewHTTPURL string `json:"PeTransferServerNewHttpUrl"`
	AuthServerURL              string `json:"AuthServerUrl"`
	AuthServerCppURL           string `json:"AuthServerCppUrl"`
	ChatServerURL              string `json:"ChatServerUrl"`
	ApiGatewayURL              string `json:"ApiGatewayUrl"`
	ApiGatewayGrayURL          string `json:"ApiGatewayGrayUrl"`
	DCWebURL                   string `json:"DCWebUrl"`
	RentalTransferURL          string `json:"RentalTransferUrl"`
}

const x19DefaultVersion = "1.15.7.20505"

// Cookie数据结构
type CookieData struct {
	SauthJSON string `json:"sauth_json"`
	MacAddr   string `json:"mac_addr"`
	RAM       string `json:"ram"`
	ROM       string `json:"rom"`
	IsGuest   *bool  `json:"is_guest"`
	Emulator  *int   `json:"emulator"`
}

func (c *CookieData) macAddress() string {
	mac := strings.ToUpper(strings.TrimSpace(c.MacAddr))
	if mac == "" {
		return "02:00:00:00:00:00"
	}
	mac = strings.ReplaceAll(mac, "-", ":")
	return mac
}

func (c *CookieData) ramValue() string {
	if strings.TrimSpace(c.RAM) == "" {
		return "4294967296"
	}
	return c.RAM
}

func (c *CookieData) romValue() string {
	return c.ROM
}

func (c *CookieData) isGuestFlag() int {
	if c.IsGuest != nil && *c.IsGuest {
		return 1
	}
	return 0
}

func (c *CookieData) emulatorFlag() int {
	if c.Emulator != nil {
		return *c.Emulator
	}
	return 0
}

type SauthData struct {
	GameID         string `json:"gameid"`
	LoginChannel   string `json:"login_channel"`
	AppChannel     string `json:"app_channel"`
	Platform       string `json:"platform"`
	SDKUID         string `json:"sdkuid"`
	SessionID      string `json:"sessionid"`
	SDKVersion     string `json:"sdk_version"`
	UDID           string `json:"udid"`
	DeviceID       string `json:"deviceid"`
	AimInfo        string `json:"aim_info"`
	ClientLoginSN  string `json:"client_login_sn"`
	GasToken       string `json:"gas_token"`
	SourcePlatform string `json:"source_platform"`
	IP             string `json:"ip"`
}

type PeAuthData struct {
	EngineVersion string         `json:"engine_version"`
	ExtraParam    string         `json:"extra_param"`
	Message       string         `json:"message"`
	PatchVersion  string         `json:"patch_version"`
	PayChannel    string         `json:"pay_channel"`
	SaData        string         `json:"sa_data"`
	SauthJSON     map[string]any `json:"sauth_json"`
	Seed          string         `json:"seed"`
	Sign          string         `json:"sign"`
}

// 使用Cookie进行PE认证
func (c *Client) G79AuthenticateWithCookie(cookieStr string) error {
	c.Cookie = cookieStr
	// 解析Cookie
	var cookieData CookieData
	err := json.Unmarshal([]byte(cookieStr), &cookieData)
	if err != nil {
		return fmt.Errorf("解析Cookie失败: %v", err)
	}

	var sauthData SauthData
	err = json.Unmarshal([]byte(cookieData.SauthJSON), &sauthData)
	if err != nil {
		return fmt.Errorf("解析SauthJSON失败: %v", err)
	}

	// 执行PE认证
	err = c.g79PerformPEAuthWithCookie(&sauthData, &cookieData)
	if err != nil {
		return fmt.Errorf("PE认证失败: %v", err)
	}

	// 获取用户详情
	userDetail, err := c.GetUserDetail()
	if err != nil {
		return fmt.Errorf("获取用户信息失败: %v", err)
	}

	c.UserDetail = &userDetail.Entity

	return nil
}

// 使用Cookie执行PE认证
func (c *Client) g79PerformPEAuthWithCookie(sauthData *SauthData, cookieData *CookieData) error {
	if sauthData == nil {
		return fmt.Errorf("缺少 sauth 数据")
	}
	if c.G79LatestVersion == "" || c.patchResourcesHash == "" {
		return fmt.Errorf("缺少补丁信息，请重新初始化客户端")
	}

	required := map[string]string{
		"sdkuid":    sauthData.SDKUID,
		"sessionid": sauthData.SessionID,
		"udid":      sauthData.UDID,
		"deviceid":  sauthData.DeviceID,
	}
	for field, val := range required {
		if strings.TrimSpace(val) == "" {
			return fmt.Errorf("sauth_json 缺少字段 %s", field)
		}
	}

	seed := uuid.New().String()
	c.Seed = seed

	clientLoginSN, err := generateClientLoginSN()
	if err != nil {
		return fmt.Errorf("生成 client_login_sn 失败: %w", err)
	}

	sauthPayload := buildAndroidSauthPayload(sauthData, clientLoginSN)

	saDataPayload := buildAndroidSaDataPayload(c, sauthData, cookieData)
	saDataJSON, err := json.Marshal(saDataPayload)
	if err != nil {
		return fmt.Errorf("序列化 sa_data 失败: %w", err)
	}

	versionMessage := fmt.Sprintf("%s%s%s%s%s%s", c.EngineVersion, g79LibraryHash, c.G79LatestVersion, c.patchResourcesHash, g79SignatureHash, seed)
	sign, err := utils.PeAuthSign(versionMessage, 2, 9)
	if err != nil {
		return fmt.Errorf("计算签名失败: %w", err)
	}

	peauth := PeAuthData{
		EngineVersion: c.EngineVersion,
		ExtraParam:    "extra",
		Message:       versionMessage,
		PatchVersion:  c.G79LatestVersion,
		PayChannel:    g79AndroidMessageTag,
		SaData:        string(saDataJSON),
		SauthJSON:     sauthPayload,
		Seed:          seed,
		Sign:          sign,
	}

	payload, err := json.Marshal(peauth)
	if err != nil {
		return fmt.Errorf("序列化认证数据失败: %w", err)
	}

	encryptedPayload, err := G79HttpEncrypt(payload)
	if err != nil {
		return fmt.Errorf("加密认证数据失败: %w", err)
	}

	req, err := http.NewRequest("POST", c.ReleaseJSON.CoreServerURL+"/pe-authentication", strings.NewReader(hex.EncodeToString(encryptedPayload)))
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return err
	}

	encryptedResp, err := hex.DecodeString(string(respBody))
	if err != nil {
		return err
	}

	decryptedResp, err := G79HttpDecrypt(encryptedResp)
	if err != nil {
		return err
	}

	validJSON := GetValidJSON(decryptedResp)

	var loginResp LoginResponse
	if err := json.Unmarshal(validJSON, &loginResp); err != nil {
		return fmt.Errorf("解析登录响应失败: %v, 响应内容: %s", err, string(validJSON))
	}

	if loginResp.Code != 0 {
		return fmt.Errorf("登录失败 (code: %d): %s", loginResp.Code, loginResp.Message)
	}

	c.SetCredentials(loginResp.Entity.EntityID, loginResp.Entity.Token)
	c.Seed = loginResp.Entity.Seed

	return nil
}

func generateClientLoginSN() (string, error) {
	buf, err := randomBytes(16)
	if err != nil {
		return "", err
	}
	return strings.ToUpper(hex.EncodeToString(buf)), nil
}

func buildAndroidSauthPayload(sauthData *SauthData, clientLoginSN string) map[string]any {
	aimInfo := strings.TrimSpace(sauthData.AimInfo)
	if aimInfo == "" {
		aimInfo = `{"aim":"127.0.0.1","country":"CN","tz":"+0800","tzid":"","celluar_ip":"","operator":"","is_vpn_enabled":"false"}`
	}
	sdkVersion := strings.TrimSpace(sauthData.SDKVersion)
	if sdkVersion == "" {
		sdkVersion = "5.9.0"
	}
	ip := strings.TrimSpace(sauthData.IP)
	if ip == "" {
		ip = "127.0.0.1"
	}
	gameID := strings.TrimSpace(sauthData.GameID)
	if gameID == "" {
		gameID = "x19"
	}
	return map[string]any{
		"aim_info":           aimInfo,
		"app_channel":        "netease",
		"client_login_sn":    clientLoginSN,
		"deviceid":           sauthData.DeviceID,
		"gameid":             gameID,
		"gas_token":          sauthData.GasToken,
		"get_access_token":   "1",
		"ip":                 ip,
		"is_unisdk_guest":    0,
		"login_channel":      "netease",
		"platform":           "ad",
		"sdk_version":        sdkVersion,
		"sdkuid":             sauthData.SDKUID,
		"sessionid":          sauthData.SessionID,
		"source_app_channel": "netease",
		"source_platform":    "ad",
		"step":               g79AndroidStep,
		"step2":              g79AndroidStep2,
		"udid":               sauthData.UDID,
	}
}

func buildAndroidSaDataPayload(c *Client, sauthData *SauthData, cookieData *CookieData) map[string]any {
	sdkVersion := strings.TrimSpace(sauthData.SDKVersion)
	if sdkVersion == "" {
		sdkVersion = "5.9.0"
	}
	udid := sauthData.UDID
	if strings.TrimSpace(udid) == "" {
		udid = "0000000000000000"
	}

	return map[string]any{
		"app_channel":   "netease",
		"app_ver":       c.G79LatestVersion,
		"core_num":      "u0004",
		"cpu_digit":     "64",
		"cpu_hz":        "2465600",
		"cpu_name":      "placeholder",
		"device_height": "900",
		"device_model":  "SAMSUNG#SM-G977N",
		"device_width":  "1600",
		"disk":          "",
		"emulator":      cookieData.emulatorFlag(),
		"first_udid":    udid,
		"is_guest":      cookieData.isGuestFlag(),
		"launcher_type": "PE_C++",
		"mac_addr":      cookieData.macAddress(),
		"network":       "mm_10086",
		"os_name":       "android",
		"os_ver":        "5.1.1",
		"ram":           cookieData.ramValue(),
		"rom":           cookieData.romValue(),
		"root":          false,
		"sdk_ver":       sdkVersion,
		"start_type":    "default",
		"udid":          udid,
	}
}

func (c *Client) G79AuthenticateWithPeAuth(peAuthHexStr string) error {
	peAuthEncryptedBytes, err := hex.DecodeString(peAuthHexStr)
	if err != nil {
		return fmt.Errorf("解析 PeAuth 失败: %v", err)
	}
	peAuthBytes, err := G79HttpDecrypt(peAuthEncryptedBytes)
	if err != nil {
		return fmt.Errorf("解密 PeAuth 失败: %v", err)
	}
	peAuthBytes = GetValidJSON(peAuthBytes)
	var peAuthData PeAuthData
	err = json.Unmarshal(peAuthBytes, &peAuthData)
	if err != nil {
		return fmt.Errorf("解析 PeAuth JSON 失败: %v", err)
	}

	// 发送认证请求
	sauthJSON, _ := json.Marshal(peAuthData.SauthJSON)
	cookieData := CookieData{
		SauthJSON: string(sauthJSON),
	}
	cookieStr, _ := json.Marshal(cookieData)
	c.Cookie = string(cookieStr)
	err = c.G79AuthenticateWithCookie(string(cookieStr))
	if err != nil {
		return fmt.Errorf("PE认证失败: %v", err)
	}
	return nil
}

type X19LoginOTPResponse struct {
	Response
	Entity X19LoginOTPEntity `json:"entity"`
}

type X19LoginOTPEntity struct {
	OTP          int    `json:"otp"`
	OTPToken     string `json:"otp_token"`
	AID          int    `json:"aid"`
	LockTime     int    `json:"lock_time"`
	OpenOTP      int    `json:"open_otp"`
	VerifyStatus int    `json:"verify_status"`
}

func (c *Client) X19AuthenticateWithCookie(cookieStr string) error {
	c.Cookie = cookieStr
	// 解析Cookie
	var cookieData CookieData
	err := json.Unmarshal([]byte(cookieStr), &cookieData)
	if err != nil {
		return fmt.Errorf("解析Cookie失败: %v", err)
	}
	// 解析 sauth_json
	var sauthData SauthData
	err = json.Unmarshal([]byte(cookieData.SauthJSON), &sauthData)
	if err != nil {
		return fmt.Errorf("解析 sauth_json 失败: %v", err)
	}

	// 发送认证请求
	req, err := http.NewRequest("POST", c.X19ReleaseJSON.CoreServerURL+"/login-otp", strings.NewReader(cookieStr))
	if err != nil {
		return err
	}

	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return err
	}
	var loginResp X19LoginOTPResponse
	if err := json.Unmarshal(respBody, &loginResp); err != nil {
		return fmt.Errorf("解析登录响应失败: %v, 响应内容: %s", err, string(respBody))
	}

	if loginResp.Code != 0 {
		return fmt.Errorf("登录失败 (code: %d): %s", loginResp.Code, loginResp.Message)
	}
	if loginResp.Entity.OTPToken == "" {
		return fmt.Errorf("登录响应缺少 otp_token")
	}

	/*
		var sauthMap map[string]any
		if err := json.Unmarshal([]byte(cookieData.SauthJSON), &sauthMap); err != nil {
			return fmt.Errorf("解析 sauth_json 失败: %v", err)
		}

		getString := func(key string) (string, error) {
			val, ok := sauthMap[key]
			if !ok {
				return "", fmt.Errorf("sauth_json 缺少字段 %s", key)
			}
			str, ok := val.(string)
			if !ok {
				return "", fmt.Errorf("sauth_json 字段 %s 类型错误", key)
			}
			return str, nil
		}

		sdkUID, err := getString("sdkuid")
		if err != nil {
			return err
		}
		sessionID, err := getString("sessionid")
		if err != nil {
			return err
		}
		udid, err := getString("udid")
		if err != nil {
			return err
		}
		deviceID, err := getString("deviceid")
		if err != nil {
			return err
		}
	*/

	saData := map[string]any{
		"os_name":       "windows",
		"os_ver":        "Microsoft Windows 10",
		"mac_addr":      "B8975A4AD616",
		"udid":          "BFEBFBFF000306A9C78C00D8",
		"app_ver":       x19DefaultVersion,
		"sdk_ver":       "",
		"network":       "",
		"disk":          "C78C00D8",
		"is64bit":       "1",
		"video_card1":   "Video_card1",
		"video_card2":   "",
		"video_card3":   "",
		"video_card4":   "",
		"launcher_type": "PC_java",
		"pay_channel":   "4399pc",
		"dotnet_ver":    "4.8.0",
		"cpu_type":      "Intel(R) Xeon(R) CPU i32100 3.10GHz",
		"ram_size":      "8553332736",
		"device_width":  "1920",
		"device_height": "1080",
		"os_detail":     "10",
	}
	saDataJSON, err := json.Marshal(saData)
	if err != nil {
		return fmt.Errorf("序列化 sa_data 失败: %v", err)
	}

	/*
		aimInfo := map[string]string{
			"code_1":  `{"code":"AS","names":{"en":"Asia"}}`,
			"code_2":  `{"iso_code":"CN","names":{"en":"China"}}`,
			"code_3":  `{"iso_code":"51","names":{"en":"Sichuan"}}`,
			"code_4":  `{"id":5101,"names":{"en":"Chengdu"}}`,
			"isp":     `{"id":10086,"names":{"en":""}}`,
			"aim":     "100.100.100.100",
			"country": "CN",
			"tz":      "+0800",
			"tzid":    "",
		}
		aimInfoJSON, err := json.Marshal(aimInfo)
		if err != nil {
			return fmt.Errorf("序列化 aim_info 失败: %v", err)
		}

		saAuthJSON := map[string]any{
			"gameid":        "x19",
			"login_channel": "netease",
			"app_channel":   "netease",
			"platform":      "pc",
			"sdkuid":        sdkUID,
			"sessionid":     sessionID,
			"sdk_version":   "4.10.0",
			"udid":          udid,
			"deviceid":      deviceID,
			"aim_info":     sauthData.AimInfo,
			"client_login_sn": sauthData.ClientLoginSN,
			"gas_token":       sauthData.GasToken,
			"source_platform": "pc",
			"ip":              sauthData.IP,
			"get_access_token": "1",
		}
		saAuthJSONBytes, err := json.Marshal(saAuthJSON)
		if err != nil {
			return fmt.Errorf("序列化 sauth_json 失败: %v", err)
		}
	*/
	authData := map[string]any{
		"sa_data":    string(saDataJSON),
		"sauth_json": cookieData.SauthJSON,
		"version": map[string]any{
			"version":      x19DefaultVersion,
			"launcher_md5": nil,
			"updater_md5":  nil,
		},
		"sdkuid":             nil,
		"aid":                strconv.Itoa(loginResp.Entity.AID),
		"hasMessage":         false,
		"hasGmail":           false,
		"otp_token":          loginResp.Entity.OTPToken,
		"otp_pwd":            nil,
		"lock_time":          loginResp.Entity.LockTime,
		"env":                nil,
		"min_engine_version": nil,
		"min_patch_version":  nil,
		"unisdk_login_json":  nil,
		"verify_status":      loginResp.Entity.VerifyStatus,
		"token":              nil,
		"is_register":        true,
		"entity_id":          nil,
	}

	authPayloadBytes, err := json.Marshal(authData)
	if err != nil {
		return fmt.Errorf("序列化认证载荷失败: %v", err)
	}
	authPayload := string(authPayloadBytes)
	encryptedAuthPayload, err := X19HttpEncrypt(authPayloadBytes)
	if err != nil {
		return fmt.Errorf("加密认证载荷失败: %v", err)
	}
	authReq, err := http.NewRequest("POST", c.X19ReleaseJSON.CoreServerURL+"/authentication-otp", bytes.NewReader(encryptedAuthPayload))
	if err != nil {
		return fmt.Errorf("创建请求失败: %v", err)
	}
	authReq.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	authReq.Header.Set("Accept-Encoding", "gzip")
	authReq.Header.Set("Content-Type", "application/json")
	authReq.Header.Set("user-id", "")
	authReq.Header.Set("user-token", CalculateDynamicToken("/authentication-otp", authPayload, ""))

	authResp, err := http.DefaultClient.Do(authReq)
	if err != nil {
		return fmt.Errorf("发送请求失败: %v", err)
	}
	defer authResp.Body.Close()

	authRespBody, err := readResponseBody(authResp)
	if err != nil {
		return fmt.Errorf("读取响应失败: %v", err)
	}

	decryptedAuthResp, err := X19HttpDecrypt(authRespBody)
	if err != nil {
		return fmt.Errorf("解密响应失败: %v", err)
	}

	validAuthRespJSON := GetValidJSON(decryptedAuthResp)
	var loginRespData LoginResponse
	if err := json.Unmarshal(validAuthRespJSON, &loginRespData); err != nil {
		return fmt.Errorf("解析登录响应失败: %v", err)
	}
	if loginRespData.Code != 0 {
		return fmt.Errorf("登录失败 (code: %d): %s", loginRespData.Code, loginRespData.Message)
	}

	// 设置用户凭证
	c.SetCredentials(loginRespData.Entity.EntityID, loginRespData.Entity.Token)
	c.Seed = loginRespData.Entity.Seed

	// 获取用户详情
	userDetail, err := c.GetUserDetail()
	if err != nil {
		return fmt.Errorf("获取用户信息失败: %v", err)
	}
	c.UserDetail = &userDetail.Entity
	c.ReleaseJSON.ApiGatewayUrl = c.X19ReleaseJSON.ApiGatewayGrayURL
	return nil
}

var OSName = "android"

// 生成租赁服认证v2数据
func (c *Client) GenerateRentalGameAuthV2(serverID, clientKey string) ([]byte, error) {
	uid, err := c.GetUserIDInt()
	if err != nil {
		return nil, err
	}

	authv2 := map[string]any{
		"bit":           "64",
		"clientKey":     clientKey,
		"displayName":   c.UserDetail.Name,
		"engineVersion": c.EngineVersion,
		"netease_sid":   fmt.Sprintf("%s:RentalGame", serverID),
		"os_name":       OSName,
		"patchVersion":  c.G79LatestVersion,
		"uid":           uid,
	}

	return json.Marshal(authv2)
}

// 生成山头认证v2数据
func (c *Client) GenerateDomainGameAuthV2(serverID, clientKey string) ([]byte, error) {
	uid, err := c.GetUserIDInt()
	if err != nil {
		return nil, err
	}

	authv2 := map[string]any{
		"bit":           "64",
		"clientKey":     clientKey,
		"displayName":   c.UserDetail.Name,
		"engineVersion": c.EngineVersion,
		"netease_sid":   fmt.Sprintf("%s:DomainGame", serverID),
		"os_name":       OSName,
		"patchVersion":  c.G79LatestVersion,
		"uid":           uid,
	}

	return json.Marshal(authv2)
}

// 生成PC山头认证v2数据
func (c *Client) GeneratePCDomainGameAuthV2(serverID, clientKey string) ([]byte, error) {
	uid, err := c.GetUserIDInt()
	if err != nil {
		return nil, err
	}

	authv2 := map[string]any{
		"bit":           "64",
		"clientKey":     clientKey,
		"displayName":   c.UserDetail.Name,
		"engineVersion": c.EngineVersion,
		"netease_sid":   fmt.Sprintf("%s:DomainGame", serverID),
		"os_name":       "windows",
		"patchVersion":  "",
		"pcCheck":       "0",
		"platform":      "pc",
		"uid":           uid,
	}

	return json.Marshal(authv2)
}

// 生成大厅游戏认证v2数据（netease_sid: roomID:LobbyGame）
func (c *Client) GenerateLobbyGameAuthV2(roomID, clientKey string) ([]byte, error) {
	uid, err := c.GetUserIDInt()
	if err != nil {
		return nil, err
	}

	authv2 := map[string]any{
		"bit":           "64",
		"clientKey":     clientKey,
		"displayName":   c.UserDetail.Name,
		"engineVersion": c.EngineVersion,
		"netease_sid":   fmt.Sprintf("%s:LobbyGame", roomID),
		"os_name":       OSName,
		"patchVersion":  c.G79LatestVersion,
		"uid":           uid,
	}

	return json.Marshal(authv2)
}

// 生成PC大厅游戏认证v2数据（netease_sid: roomID:LobbyGame）
func (c *Client) GeneratePCLobbyGameAuthV2(roomID, clientKey string) ([]byte, error) {
	uid, err := c.GetUserIDInt()
	if err != nil {
		return nil, err
	}

	authv2 := map[string]any{
		"bit":           "64",
		"clientKey":     clientKey,
		"displayName":   c.UserDetail.Name,
		"engineVersion": c.EngineVersion,
		"netease_sid":   fmt.Sprintf("%s:LobbyGame", roomID),
		"os_name":       "windows",
		"patchVersion":  "",
		"pcCheck":       "0",
		"platform":      "pc",
		"uid":           uid,
	}

	return json.Marshal(authv2)
}

// 生成网络游戏认证v2数据（netease_sid: roomID:NetworkGame）
func (c *Client) GenerateNetworkGameAuthV2(roomID, clientKey string) ([]byte, error) {
	uid, err := c.GetUserIDInt()
	if err != nil {
		return nil, err
	}

	authv2 := map[string]any{
		"bit":           "64",
		"clientKey":     clientKey,
		"displayName":   c.UserDetail.Name,
		"engineVersion": c.EngineVersion,
		"netease_sid":   fmt.Sprintf("%s:NetworkGame", roomID),
		"os_name":       OSName,
		"patchVersion":  c.G79LatestVersion,
		"uid":           uid,
	}

	return json.Marshal(authv2)
}

// 发送认证v2请求
func (c *Client) SendAuthV2Request(authv2Data []byte) ([]byte, error) {
	api := "/authentication-v2"

	encryptedData, err := G79HttpEncrypt(authv2Data)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", c.ReleaseJSON.AuthServerURL+api, strings.NewReader(hex.EncodeToString(encryptedData)))
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("user-id", c.UserID)

	token := CalculateDynamicToken(api, string(authv2Data), c.UserToken)
	req.Header.Set("user-token", hex.EncodeToString([]byte(token)))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, err
	}

	encryptedResp, err := hex.DecodeString(string(respBody))
	if err != nil {
		return nil, err
	}

	decryptedResp, err := G79HttpDecrypt(encryptedResp)
	if err != nil {
		return nil, err
	}

	return GetValidJSON(decryptedResp), nil
}
