// rawrelay 测试：go-raknet 连接 Geyser 后，原样透传数据（不经过 minecraft 层）。
// 验证 go-raknet Conn 能否直接读写原始 RakNet 数据（相当于透传通道）。
package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/sandertv/go-raknet"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := raknet.DialContext(ctx, "example.com:49780")
	if err != nil {
		fmt.Printf("raknet 失败: %v\n", err)
		return
	}
	defer conn.Close()
	fmt.Println("go-raknet 连接成功:", conn.RemoteAddr())

	// 尝试读取（Geyser 会先发 Server To Client Handshake 等包）
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		if err == io.EOF {
			fmt.Println("EOF（连接关闭）")
		} else {
			fmt.Printf("读取: %v\n", err)
		}
		return
	}
	fmt.Printf("收到 %d 字节: %x\n", n, buf[:n])
}
