// deflateprobe 用原始 deflate 解压一段 hex（去掉可选前缀字节），打印解压结果。
// 用法: deflateprobe <hex> [<前缀字节数>]
package main

import (
	"bytes"
	"compress/flate"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: deflateprobe <hex> [<skip-bytes>]")
		return
	}
	h := strings.ReplaceAll(os.Args[1], " ", "")
	// 去掉所有非 hex 字符（换行、引号等）
	clean := make([]rune, 0, len(h))
	for _, c := range h {
		if c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F' {
			clean = append(clean, c)
		}
	}
	h = string(clean)
	if len(h)%2 != 0 {
		h = h[:len(h)-1]
	}
	b, err := hex.DecodeString(h)
	if err != nil {
		fmt.Println("hex 解析失败:", err)
		return
	}
	skip := 0
	if len(os.Args) > 2 {
		fmt.Sscanf(os.Args[2], "%d", &skip)
	}
	if skip > 0 {
		b = b[skip:]
	}
	fmt.Printf("输入 %d 字节 (跳过 %d): %x\n", len(b), skip, b)

	r := flate.NewReader(bytes.NewReader(b))
	out := make([]byte, 0, 65536)
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		out = append(out, buf[:n]...)
		if err != nil {
			fmt.Printf("解压读取结束 (err=%v)，共解出 %d 字节\n", err, len(out))
			break
		}
	}
	fmt.Printf("解压结果(前512字节): %x\n", out[:min(len(out), 512)])
	// 尝试打印可见字符
	vis := make([]rune, 0, len(out))
	for _, c := range out {
		if c >= 0x20 && c < 0x7f {
			vis = append(vis, rune(c))
		} else if c == '\n' {
			vis = append(vis, '|')
		} else {
			vis = append(vis, '.')
		}
	}
	if len(vis) > 400 {
		vis = vis[:400]
	}
	fmt.Printf("可见字符: %s\n", string(vis))
}
