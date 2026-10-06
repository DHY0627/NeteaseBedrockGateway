// logindump 解析玩家 Login 消息：跳算法字节，raw-deflate 解压，解析批次帧与 Login 包，打印 JSON 结构概览。
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
	if len(os.Args) < 2 {
		fmt.Println("usage: logindump <hexfile>")
		return
	}
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

	// 玩家 Login = [algo 0x00][raw-deflate]
	r := flate.NewReader(bytes.NewReader(b[1:]))
	out, _ := io.ReadAll(r)
	fmt.Printf("解压 %d -> %d 字节\n", len(b)-1, len(out))

	// 解析批次帧 [varint len][frame...]
	i := 0
	for i < len(out) {
		fl, n := readVarInt(out[i:])
		if n == 0 || i+n+int(fl) > len(out) {
			fmt.Printf("（帧解析停止 at %d）\n", i)
			break
		}
		frame := out[i+n : i+n+int(fl)]
		i += n + int(fl)
		hd, hn := readVarInt(frame)
		if hn == 0 {
			continue
		}
		fmt.Printf("帧: 长度=%d 帧头varint=%d packetId=%d\n", fl, hd, hd&0x3ff)
		payload := frame[hn:]
		if (hd & 0x3ff) == 1 { // Login
			// Login payload: [varint chainLen][chain string][skin string...]
			cl, cn := readVarInt(payload)
			if cn == 0 || cn+int(cl) > len(payload) {
				fmt.Println("  链长度解析失败")
				continue
			}
			chainStr := string(payload[cn : cn+int(cl)])
			rest := payload[cn+int(cl):]
			fmt.Printf("  链: 长度=%d, 开头=%s...\n", cl, chainStr[:min(80, len(chainStr))])
			var chainJSON map[string]interface{}
			json.Unmarshal([]byte(chainStr), &chainJSON)
			if cert, ok := chainJSON["Certificate"].(string); ok {
				var certObj map[string]interface{}
				json.Unmarshal([]byte(cert), &certObj)
				if ch, ok := certObj["chain"].([]interface{}); ok {
					fmt.Printf("  chain 元素数: %d\n", len(ch))
					for idx, c := range ch {
						cs := c.(string)
						parts := strings.Split(cs, ".")
						dh, _ := base64Decode(parts[0])
						dp, _ := base64Decode(parts[1])
						fmt.Printf("    chain[%d] header: %s\n", idx, string(dh)[:min(120, len(dh))])
						fmt.Printf("    chain[%d] payload: %s\n", idx, string(dp)[:min(220, len(dp))])
					}
				}
			}
			fmt.Printf("  其余(%d字节) 开头: %x\n", len(rest), rest[:min(40, len(rest))])
			if len(rest) > 4 {
				sl, sn := readVarInt(rest)
				if sn > 0 && sn+int(sl) <= len(rest) {
					fmt.Printf("  skin JWT 长度=%d: %s...\n", sl, string(rest[sn:sn+int(min(60, sl))]))
				}
			}
		}
	}
}

func readVarInt(b []byte) (uint32, int) {
	var v uint32
	var shift uint
	for i := 0; i < len(b) && i < 5; i++ {
		c := b[i]
		v |= uint32(c&0x7f) << shift
		if c&0x80 == 0 {
			return v, i + 1
		}
		shift += 7
	}
	return 0, 0
}

func base64Decode(s string) ([]byte, error) {
	if d, err := base64.StdEncoding.DecodeString(s); err == nil {
		return d, nil
	}
	return base64.RawURLEncoding.DecodeString(s)
}
