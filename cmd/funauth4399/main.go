// funauth4399 通过 4399 账号密码登录我的世界中国版（x19），
// 并完成 FunAuth 的"创建房间"（TanLobbyCreate）流程，输出开房凭据。
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"NeteaseBedrockGateway/internal/auth"
	"NeteaseBedrockGateway/internal/room"
)

func main() {
	var (
		username = flag.String("u", "", "4399 用户名")
		password = flag.String("p", "", "4399 密码")
		timeout  = flag.Duration("timeout", 180*time.Second, "总超时时间")
	)
	flag.Parse()

	if *username == "" {
		fmt.Fprintln(os.Stderr, "错误: 必须提供 4399 用户名 (-u)")
		flag.Usage()
		os.Exit(2)
	}
	if *password == "" {
		fmt.Fprintln(os.Stderr, "错误: 必须提供 4399 密码 (-p)")
		flag.Usage()
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	fmt.Fprintln(os.Stderr, "[1/3] 4399 登录 + x19 认证中 ...")
	cli, cookie, err := auth.Login(ctx, *username, *password, auth.LoginOptions{
		OnCaptcha: interactiveCaptchaHandler,
	})
	if err != nil {
		fatalf("%v", err)
	}
	fmt.Fprintf(os.Stderr, "[1/3] 认证成功: sdkuid=%s uid=%s\n", cookie.SDKUid, cli.UserID)

	fmt.Fprintln(os.Stderr, "[2/3] 创建房间（TanLobbyCreate）...")
	result, err := room.Create(ctx, cli)
	if err != nil {
		fatalf("创建房间失败: %v", err)
	}

	fmt.Fprintln(os.Stderr, "[3/3] 输出开房凭据 ...")
	out, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fatalf("序列化结果失败: %v", err)
	}
	fmt.Println(string(out))
	fmt.Fprintln(os.Stderr, "完成。")
}

// interactiveCaptchaHandler 遇到图形验证码时，将图片保存到本地文件并提示用户在终端输入。
func interactiveCaptchaHandler(image []byte) (string, error) {
	const path = "captcha.png"
	if err := os.WriteFile(path, image, 0o644); err != nil {
		return "", fmt.Errorf("保存验证码图片失败: %w", err)
	}
	fmt.Fprintf(os.Stderr, "请查看验证码图片 %s 并输入其中的字符: ", path)
	code, err := readLine()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(code), nil
}

func readLine() (string, error) {
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "错误: "+format+"\n", args...)
	os.Exit(1)
}
