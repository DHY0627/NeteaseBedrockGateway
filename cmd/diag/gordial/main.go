// gordial 用 sandertv/go-raknet 测试连接 Geyser（be.4f4t.top:49780）。
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/sandertv/go-raknet"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	start := time.Now()
	conn, err := raknet.DialContext(ctx, "be.4f4t.top:49780")
	if err != nil {
		fmt.Printf("go-raknet Dial 失败: %v (耗时 %v)\n", err, time.Since(start))
		return
	}
	fmt.Printf("go-raknet Dial 成功 (耗时 %v) %v\n", time.Since(start), conn.RemoteAddr())
	conn.Close()
}
