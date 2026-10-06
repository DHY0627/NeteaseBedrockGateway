// logextract 从 host.log 提取指定消息的完整 hex（拼接被管道换行的片段），输出到文件。
package main

import (
	"fmt"
	"os"
	"strings"
	"unicode/utf16"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("usage: logextract <logfile> <字节标记> [outfile]")
		return
	}
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Println("read:", err)
		return
	}
	var text string
	// 检测 UTF-16 BOM
	if len(data) >= 2 && data[0] == 0xff && data[1] == 0xfe {
		decoded, err := decodeUTF16(data[2:])
		if err != nil {
			fmt.Println("utf16 decode:", err)
			return
		}
		text = decoded
	} else {
		text = string(data)
	}
	marker := os.Args[2]

	// 逐行扫描，收集所有 hex 片段
	lines := strings.Split(text, "\n")
	var buf strings.Builder
	inMsg := false
	count := 0
	for _, ln := range lines {
		trimmed := strings.TrimSpace(ln)
		if strings.Contains(trimmed, marker) && strings.Contains(trimmed, "字节:") {
			// 开始新消息：截取 "字节: " 之后
			idx := strings.Index(trimmed, "字节: ")
			if idx >= 0 {
				if buf.Len() > 0 && inMsg {
					fmt.Printf("（拼接完成 %d）\n", count)
				}
				buf.Reset()
				inMsg = true
				count++
				rest := trimmed[idx+len("字节: "):]
				rest = strings.TrimRight(rest, "\"")
				buf.WriteString(rest)
			}
			continue
		}
		if inMsg {
			// 纯 hex 行继续拼接；遇到 time=/At line/错误块则结束
			if len(trimmed) > 0 && isHex(trimmed) {
				buf.WriteString(trimmed)
				continue
			}
			// 结束
			if buf.Len() > 0 {
				out := buf.String()
				if len(os.Args) > 3 {
					os.WriteFile(os.Args[3]+fmt.Sprintf("_%d.hex", count), []byte(out), 0644)
					fmt.Printf("第 %d 条已写入 %s_%d.hex，hex 长度=%d\n", count, os.Args[3], count, len(out))
				} else {
					fmt.Printf("第 %d 条 hex 长度=%d: %s...\n", count, len(out), out[:min(80, len(out))])
				}
				buf.Reset()
			}
			inMsg = false
		}
	}
	if inMsg && buf.Len() > 0 {
		out := buf.String()
		if len(os.Args) > 3 {
			os.WriteFile(os.Args[3]+fmt.Sprintf("_%d.hex", count), []byte(out), 0644)
			fmt.Printf("第 %d 条已写入 %s_%d.hex，hex 长度=%d\n", count, os.Args[3], count, len(out))
		}
	}
}

func isHex(s string) bool {
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return len(s) > 0
}

func decodeUTF16(b []byte) (string, error) {
	if len(b)%2 != 0 {
		b = b[:len(b)-1]
	}
	u := make([]uint16, len(b)/2)
	for i := 0; i < len(u); i++ {
		u[i] = uint16(b[2*i]) | uint16(b[2*i+1])<<8
	}
	return string(utf16.Decode(u)), nil
}
