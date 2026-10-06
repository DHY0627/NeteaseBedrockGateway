package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// LeaveMainCityResponse is returned when calling /leave-main-city.
type LeaveMainCityResponse struct {
	Response
	Details string          `json:"details"`
	Entity  json.RawMessage `json:"entity"`
}

// LeaveMainCity clears the main city session for the current user.
func (c *Client) LeaveMainCity() (*LeaveMainCityResponse, error) {
	api := "/leave-main-city"
	body, err := json.Marshal(map[string]any{})
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

	var result LeaveMainCityResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decode leave main city response: %w", err)
	}

	return &result, nil
}
