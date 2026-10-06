// x5ucheck 对比客户端 Login 链中的 x5u 与服务器握手 x5u 的结构。
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
		fmt.Println("usage: x5ucheck <login-hex-file>")
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
	// Login 载荷：可能 [00][deflate] 或 [deflate]
	start := 0
	if b[0] == 0x00 {
		start = 1
	}
	r := flate.NewReader(bytes.NewReader(b[start:]))
	out, _ := io.ReadAll(r)
	fmt.Printf("解压 %d -> %d 字节\n", len(b)-start, len(out))

	// 找帧：[varint len][frame...]，frame=[varint header][payload]
	i := 0
	for i < len(out) {
		fl, n := readVarInt(out[i:])
		if n == 0 || i+n+int(fl) > len(out) {
			break
		}
		frame := out[i+n : i+n+int(fl)]
		i += n + int(fl)
		hd, hn := readVarInt(frame)
		if hn == 0 || (hd&0x3ff) != 1 {
			continue // 只看 Login
		}
		payload := frame[hn:]
		// 直接搜索转义形式的 `chain\":[\"` 之后的 JWT 串
		marker := []byte(`chain\":[\"`)
		mi := bytes.Index(payload, marker)
		if mi < 0 {
			continue
		}
		rest := payload[mi+len(marker):]
		// 提取每个 JWT（直到未转义的 '"'），解码 header 打印 x5u
		count := 0
		for count < 6 && len(rest) > 0 {
			end := bytes.IndexByte(rest, '"')
			if end <= 0 {
				break
			}
			tok := string(rest[:end])
			rest = rest[end+1:]
			parts := strings.Split(tok, ".")
			if len(parts) < 1 {
				break
			}
			dh, err := base64.RawURLEncoding.DecodeString(parts[0])
			if err != nil {
				break
			}
			var header map[string]interface{}
			if json.Unmarshal(dh, &header) != nil {
				break
			}
			x5u, _ := header["x5u"].(string)
			xb, err := base64.StdEncoding.DecodeString(x5u)
			if err != nil {
				xb, _ = base64.RawURLEncoding.DecodeString(x5u)
			}
			fmt.Printf("chain[%d] alg=%v x5u(%d字节): %x\n", count, header["alg"], len(xb), xb[:min(48, len(xb))])
			count++
			// 跳到下一个 JWT 或退出（下一个以 \\\" 开头或已是字符串尾）
			if len(rest) > 1 && rest[0] == ',' {
				rest = rest[1:]
				if len(rest) > 1 && rest[0] == '\\' && rest[1] == '"' {
					rest = rest[2:]
				} else if len(rest) > 1 && rest[0] == '"' {
					rest = rest[1:]
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
