// mctest 用 go-raknet + solver minecraft 连接 Geyser，验证完整握手。
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/Happy2018new/nemc-tan-lobby-solver/minecraft"
	"github.com/sandertv/go-raknet"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	netConn, err := raknet.DialContext(ctx, "be.4f4t.top:49780")
	if err != nil {
		fmt.Printf("raknet 失败: %v\n", err)
		return
	}
	defer netConn.Close()
	fmt.Println("raknet 连接成功")

	start := time.Now()
	serverConn, err := minecraft.DialContext(ctx, netConn)
	if err != nil {
		fmt.Printf("minecraft 握手失败: %v (耗时 %v)\n", err, time.Since(start))
		return
	}
	defer serverConn.Close()
	fmt.Printf("minecraft 握手成功 (耗时 %v): 游戏=%q\n", time.Since(start), serverConn.GameData().WorldName)
}
