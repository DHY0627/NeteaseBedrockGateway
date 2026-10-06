// jwtdump 解析握手响应：跳过 FE+00，raw-deflate 解压，提取 JWT 并解码 header/payload，hexdump x5u。
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
		fmt.Println("usage: jwtdump <hexfile>")
		return
	}
	raw, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Println("read:", err)
		return
	}
	h := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F' {
			return r
		}
		return -1
	}, string(raw))
	if len(h)%2 != 0 {
		h = h[:len(h)-1]
	}
	b, err := hex.DecodeString(h)
	if err != nil {
		fmt.Println("hex:", err)
		return
	}
	// 跳过 FE(0) + algo(1)
	r := flate.NewReader(bytes.NewReader(b[2:]))
	out, _ := io.ReadAll(r)
	fmt.Printf("解压 %d -> %d 字节\n", len(b)-2, len(out))

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
		fmt.Printf("帧: 长度=%d 帧头varint=%d packetId=%d 载荷=%d字节\n", fl, hd, hd&0x3ff, len(frame)-hn)
		payload := frame[hn:]
		// 尝试作为 JWT
		if idx := bytes.Index(payload, []byte("eyJ")); idx >= 0 {
			jwt := string(payload[idx:])
			parts := strings.Split(jwt, ".")
			if len(parts) >= 2 {
				dh, _ := base64.RawURLEncoding.DecodeString(parts[0])
				dp, _ := base64.RawURLEncoding.DecodeString(parts[1])
				fmt.Printf("JWT header: %s\n", dh)
				fmt.Printf("JWT payload: %s\n", dp)
				// 解码 x5u
				var header map[string]interface{}
				json.Unmarshal(dh, &header)
				if x5u, ok := header["x5u"].(string); ok {
					xb, err := base64.StdEncoding.DecodeString(x5u)
					if err != nil {
						xb, _ = base64.RawURLEncoding.DecodeString(x5u)
					}
					fmt.Printf("x5u (%d字节): %x\n", len(xb), xb[:min(48, len(xb))])
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
