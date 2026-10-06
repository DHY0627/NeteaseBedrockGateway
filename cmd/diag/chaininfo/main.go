// chaininfo 解析 Login 链 JSON：元素数量、每个元素的 header/payload 结构。
package main

import (
	"bytes"
	"compress/flate"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

func main() {
	raw, _ := os.ReadFile(os.Args[1])
	h := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F' {
			return r
		}
		return -1
	}, string(raw))
	if len(h)%2 != 0 {
		h = h[:len(h)-1]
	}
	b, _ := hex.DecodeString(h)
	start := 0
	if b[0] == 0x00 {
		start = 1
	}
	r := flate.NewReader(bytes.NewReader(b[start:]))
	out, _ := io.ReadAll(r)
	fmt.Printf("解压 %d -> %d 字节\n", len(b)-start, len(out))

	// 找 chain JSON：`{"AuthenticationType"` 开始处往前找到 JSON 起点
	idx := bytes.Index(out, []byte(`{"AuthenticationType"`))
	if idx < 0 {
		fmt.Println("未找到 AuthenticationType JSON")
		return
	}
	// 尝试解析 JSON：逐步截断找到完整 JSON
	var loginJSON map[string]interface{}
	for end := len(out); end > idx; end-- {
		if json.Unmarshal(out[idx:end], &loginJSON) == nil {
			break
		}
	}
	if loginJSON == nil {
		fmt.Println("JSON 解析失败")
		return
	}
	certStr, _ := loginJSON["Certificate"].(string)
	fmt.Printf("AuthenticationType=%v Certificate 长度=%d\n", loginJSON["AuthenticationType"], len(certStr))
	var certObj map[string]interface{}
	if err := json.Unmarshal([]byte(certStr), &certObj); err != nil {
		fmt.Println("Certificate 解析失败:", err)
		return
	}
	chain, _ := certObj["chain"].([]interface{})
	fmt.Printf("★ chain 元素数: %d\n", len(chain))
	for i, c := range chain {
		cs := c.(string)
		parts := strings.Split(cs, ".")
		fmt.Printf("--- chain[%d] 长度=%d, 段数=%d ---\n", i, len(cs), len(parts))
		if len(parts) >= 2 {
			dh, _ := base64.RawURLEncoding.DecodeString(strings.TrimSpace(parts[0]))
			dp, _ := base64.RawURLEncoding.DecodeString(strings.TrimSpace(parts[1]))
			fmt.Printf("  header: %s\n", string(dh))
			p := string(dp)
			if len(p) > 300 {
				p = p[:300] + "..."
			}
			fmt.Printf("  payload: %s\n", p)
		}
	}
	// 其余字段
	for k, v := range loginJSON {
		if k == "Certificate" {
			continue
		}
		s := fmt.Sprintf("%v", v)
		if len(s) > 120 {
			s = s[:120] + "..."
		}
		fmt.Printf("其他字段 %s = %s\n", k, s)
	}
}
