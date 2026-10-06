package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// EnterMainCityResponse describes the payload returned by /enter-main-city.
type EnterMainCityResponse struct {
	Response
	Details string              `json:"details"`
	Entity  EnterMainCityEntity `json:"entity"`
}

// EnterMainCityEntity contains the resolved server endpoints for the target version.
type EnterMainCityEntity struct {
	Code             int    `json:"code"`
	CityNo           int    `json:"city_no"`
	ResID            string `json:"res_id"`
	Version          string `json:"version"`
	CityTag          string `json:"city_tag"`
	ISPEnable        bool   `json:"isp_enable"`
	ServerHost       string `json:"server_host"`
	ServerPort       int    `json:"server_port"`
	ServerHostV6     string `json:"server_host_v6"`
	CMCCServerHost   string `json:"cmcc_server_host"`
	CTCCServerHost   string `json:"ctcc_server_host"`
	CUCCServerHost   string `json:"cucc_server_host"`
	CMCCServerPort   int    `json:"cmcc_server_port"`
	CTCCServerPort   int    `json:"ctcc_server_port"`
	CUCCServerPort   int    `json:"cucc_server_port"`
	CMCCServerHostV6 string `json:"cmcc_server_host_v6"`
	CTCCServerHostV6 string `json:"ctcc_server_host_v6"`
	CUCCServerHostV6 string `json:"cucc_server_host_v6"`
}

// EnterMainCity posts the desired client version to the ApiGateway enter-main-city endpoint.
func (c *Client) EnterMainCity() (*EnterMainCityResponse, error) {
	api := "/enter-main-city"
	payload := map[string]any{
		"version": GameVersion,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", c.ReleaseJSON.ApiGatewayUrl+api, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("User-Agent", "WPFLauncher/0.0.0.0")
	req.Header.Set("user-id", c.UserID)

	token := CalculateDynamicToken(api, string(body), c.UserToken)
	req.Header.Set("user-token", token)
	req.Header.Set("Accept-Encoding", "gzip")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, err
	}

	var result EnterMainCityResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decode enter main city response: %w", err)
	}

	return &result, nil
}
