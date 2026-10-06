// rakdial 测试 Go raknet 客户端连接 Geyser 服务器（be.4f4t.top:49780）。
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/Happy2018new/nemc-tan-lobby-solver/core/raknet"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	start := time.Now()
	conn, err := raknet.DialContext(ctx, "be.4f4t.top:49780")
	elapsed := time.Since(start)
	if err != nil {
		fmt.Printf("raknet.Dial 失败: %v (耗时 %v)\n", err, elapsed)
		return
	}
	fmt.Printf("raknet.Dial 成功 (耗时 %v) %v\n", elapsed, conn.RemoteAddr())
	conn.Close()
}
