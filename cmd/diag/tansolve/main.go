// tansolve 用 solver 的 packet pool 解码网易 TanLobby 包（尝试明文/加密两种）。
// 用法: tansolve <hex>
package main

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"github.com/Happy2018new/nemc-tan-lobby-solver/protocol/encoding"
	"github.com/Happy2018new/nemc-tan-lobby-solver/protocol/packet"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "用法: tansolve <hex>")
		os.Exit(2)
	}
	h := strings.ReplaceAll(os.Args[1], " ", "")
	b, err := hex.DecodeString(h)
	if err != nil {
		fmt.Println("hex 错误:", err)
		return
	}
	// 去掉 fee301 头
	if len(b) >= 3 && b[0] == 0xfe && b[1] == 0xe3 && b[2] == 0x01 {
		b = b[3:]
		fmt.Println("去掉 fee301 头")
	}

	buf := bytes.NewBuffer(b)
	reader := encoding.NewReader(buf)
	header := packet.Header{}
	if err := header.Read(buf); err != nil {
		fmt.Println("读 header 失败:", err)
		return
	}
	fmt.Printf("包 ID=%d\n", header.PacketID)
	pool := packet.NewClientPool()
	pk := pool[header.PacketID]
	if pk == nil {
		// 尝试服务器池
		pool = packet.NewServerPool()
		pk = pool[header.PacketID]
		if pk != nil {
			fmt.Println("(服务器池包)")
		}
	}
	if pk == nil {
		fmt.Println("未知包 ID")
		return
	}
	pk.Marshal(reader)
	fmt.Printf("%T = %+v\n", pk, pk)
}
