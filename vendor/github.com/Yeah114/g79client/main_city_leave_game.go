package g79client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// LeaveEnteredGame sends a telemetry log to drpf service to mark the main city exit event.
// It returns an error if the request fails or a non-OK response is received.
func (c *Client) LeaveEnteredGame() error {
	if c.UserID == "" {
		return fmt.Errorf("leave entered game: missing user id")
	}

	if c.UserDetail == nil {
		if detail, err := c.GetUserDetail(); err == nil {
			c.UserDetail = &detail.Entity
		}
	}

	roleName := c.UserID
	if c.UserDetail != nil && c.UserDetail.Name != "" {
		roleName = c.UserDetail.Name
	}

	sessionID := fmt.Sprintf("%s%d", c.UserID, time.Now().UnixNano())

	payload := map[string]any{
		"is_emulator":       "None",
		"jf_gameid":         "None",
		"ip":                "None",
		"common_int2":       0,
		"game_session_id":   sessionID,
		"common_int3":       0,
		"app_ver":           c.EngineVersion,
		"country_code":      86,
		"uid":               c.UserID,
		"device_level":      2,
		"transid":           "None",
		"operate_type":      "leave_main_city_no_game",
		"isp_name":          "default",
		"sead":              c.Seed,
		"os_ver":            "11",
		"network":           "CHANNEL_UNKNOW",
		"login_channel":     "netease",
		"oaid":              "None",
		"app_channel":       "netease",
		"role_id":           c.UserID,
		"source":            "netease_p2",
		"patch_ver":         c.G79LatestVersion,
		"msg":               "",
		"engine_ver":        c.EngineVersion,
		"extra_info":        "{}",
		"type":              "user_log",
		"location":          "None",
		"common_int1":       0,
		"testFlag":          false,
		"os_name":           OSName,
		"account_id":        "",
		"is_root":           "None",
		"role_name":         roleName,
		"imei":              "None",
		"WebServerUrl":      c.ReleaseJSON.WebServerUrl,
		"common_str2":       "",
		"sub_type":          "",
		"common_str1":       "",
		"main_type":         "main_city",
		"device_model":      "UNKNOWN",
		"patchVersion":      "None",
		"server":            c.ReleaseJSON.CoreServerURL,
		"project":           "g79",
		"udid":              c.UserID,
		"caid":              "None",
		"mac_addr":          "02:00:00:00:00:00",
		"account_user_name": "None",
		"error_code":        "",
		"common_str3":       "",
		"launch_type":       "cocos",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("leave entered game: %w", err)
	}

	req, err := http.NewRequest("POST", "https://drpf-g79.proxima.nie.netease.com", strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("leave entered game: %w", err)
	}

	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Content-Type", "text/plain")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("leave entered game: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("leave entered game: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("leave entered game: unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	if strings.TrimSpace(string(respBody)) != "ok" {
		return fmt.Errorf("leave entered game: unexpected response %q", strings.TrimSpace(string(respBody)))
	}

	return nil
}
