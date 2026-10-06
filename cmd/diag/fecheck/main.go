// fecheck 测试「补上 0xFE 帧头」假设：向 example.com:19132 发送
// FE + 网易客户端首个消息（RequestNetworkSettings, 协议 860），
// 观察 Geyser 是否返回正常响应而不是断开。
package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/sandertv/go-raknet"
)

func main() {
	ver := 860
	if len(os.Args) > 1 {
		fmt.Sscanf(os.Args[1], "%d", &ver)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	conn, err := raknet.DialContext(ctx, "example.com:19132")
	if err != nil {
		fmt.Printf("dial 失败: %v\n", err)
		return
	}
	defer conn.Close()
	fmt.Printf("RakNet 已连接 (protocol 8 fork), 测试协议版本 %d\n", ver)

	// 网易客户端首个 NetherNet 消息结构：
	// 06 = 批次帧长(6), c1 01 = varint 193 = RequestNetworkSettings(compat),
	// 后 4 字节 = int 协议版本
	firstMsg := []byte{0x06, 0xc1, 0x01, byte(ver >> 24), byte(ver >> 16), byte(ver >> 8), byte(ver)}
	withFE := append([]byte{0xFE}, firstMsg...)
	fmt.Printf("发送: %s\n", hex.EncodeToString(withFE))

	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	if _, err := conn.Write(withFE); err != nil {
		fmt.Printf("write 失败: %v\n", err)
		return
	}

	buf := make([]byte, 65536)
	for i := 0; i < 8; i++ {
		n, err := conn.Read(buf)
		if err != nil {
			if err == io.EOF {
				fmt.Println("连接被关闭 (EOF)")
			} else {
				fmt.Printf("read 失败: %v\n", err)
			}
			return
		}
		fmt.Printf("[响应 %d] %d 字节: %s\n", i+1, n, hex.EncodeToString(buf[:n]))
		if n > 0 && buf[0] == 0xFE {
			// 解析批次：跳过 FE，读 varint 帧长
			rest := buf[1:n]
			fl, l := readVarInt(rest)
			if l > 0 && int(fl)+l <= len(rest) {
				frame := rest[l : l+int(fl)]
				h, hl := readVarInt(frame)
				if hl > 0 {
					fmt.Printf("  批次帧长=%d 帧头varint=%d packetId=%d payload=%s\n",
						fl, h, h&0x3ff, hex.EncodeToString(frame[hl:]))
				}
			}
		}
		if n < len(buf) && n < 300 {
			// 小消息可能是最终响应，等一会再看下一条
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
