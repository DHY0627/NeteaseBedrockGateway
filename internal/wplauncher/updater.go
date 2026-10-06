package wplauncher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const patchlistURL = "https://x19.update.netease.com/pl/x19_java_patchlist"

// LatestInfo 对应 Kotlin 端 LatestInfo
type LatestInfo struct {
	Version string
	URL     string
	MD5     string
}

type patchInfo struct {
	MD5 string `json:"md5"`
	URL string `json:"url"`
}

// FetchLatestVersion 复刻 WPLUpdaterAPI.fetch
// 响应体形如: "1.20.12.02":{"md5":"..","url":".."},... , 去除尾部逗号后包裹成 JSON 对象,
// 文档中最后一个 key 即最新版本
func FetchLatestVersion(ctx context.Context, client *http.Client) (*LatestInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, patchlistURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "WPFLauncher/0.0.0.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("获取版本列表失败: %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// 去除尾部空白与逗号后包裹
	s := strings.TrimSpace(string(body))
	s = strings.TrimSuffix(s, ",")
	jsonStr := "{" + s + "}"

	// 用 Decoder 按文档顺序遍历, 取最后一个 key
	dec := json.NewDecoder(strings.NewReader(jsonStr))
	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("解析版本列表失败: %w", err)
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, fmt.Errorf("解析版本列表失败: 不是 JSON 对象")
	}

	patches := map[string]patchInfo{}
	var lastKey string
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("解析版本列表失败: %w", err)
		}
		lastKey, _ = keyTok.(string)
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, fmt.Errorf("解析版本列表失败: %w", err)
		}
		var p patchInfo
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, fmt.Errorf("解析版本列表失败: %w", err)
		}
		patches[lastKey] = p
	}

	if lastKey == "" {
		return nil, errors.New("解析版本列表失败: 未找到版本")
	}
	patch := patches[lastKey]
	return &LatestInfo{Version: lastKey, URL: patch.URL, MD5: patch.MD5}, nil
}
