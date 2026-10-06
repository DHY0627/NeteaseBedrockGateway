package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	funauth "NeteaseBedrockGateway/internal/auth"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cli, _, err := funauth.Login(ctx, os.Args[1], os.Args[2], funauth.LoginOptions{})
	if err != nil {
		fmt.Printf("认证失败: %v\n", err)
		os.Exit(1)
	}
	resp, err := cli.GetTransferRoomWithName(os.Args[3])
	if err != nil {
		fmt.Printf("查询失败: %v\n", err)
		os.Exit(1)
	}
	b, _ := json.MarshalIndent(resp, "", "  ")
	fmt.Println(string(b))
}
