// relaydecode：把 relay.log 里记录的原始 hex 解码成 Bedrock 包列表。
//
// 用法：go run ./cmd/relaydecode [relay.log]
package main

import (
	"bufio"
	"bytes"
	"compress/flate"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

func main() {
	path := "relay.log"
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "打开失败:", err)
		os.Exit(1)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	line := 0
	for sc.Scan() {
		line++
		raw := sc.Text()
		idx := strings.Index(raw, ": ")
		if idx < 0 {
			continue
		}
		head := raw[:idx]
		hexStr := strings.TrimSpace(raw[idx+2:])
		data, err := hex.DecodeString(hexStr)
		if err != nil {
			fmt.Printf("#%d [非hex] %s\n", line, head)
			continue
		}
		dir := "?"
		switch {
		case strings.Contains(head, "\u73a9\u5bb6\u2192"): // 玩家→
			dir = "玩家→服务器"
		case strings.Contains(head, "\u670d\u52a1\u5668\u2192"): // 服务器→
			dir = "服务器→玩家"
		}
		fmt.Printf("\n#%d %s len=%d\n", line, dir, len(data))
		if len(data) > 200 {
			fmt.Printf("    原始(前64)=%x...\n", data[:64])
		} else {
			fmt.Printf("    原始=%s\n", hexStr)
		}

		body := data
		// RakNet 帧头
		if len(body) > 0 && body[0] == 0xFE {
			body = body[1:]
			fmt.Printf("    RakNet帧: 0xFE 去除\n")
		}
		if len(body) == 0 {
			continue
		}
		// 批次压缩头
		switch body[0] {
		case 0x00:
			out, err := inflate(body[1:])
			if err != nil {
				fmt.Printf("    压缩头=0x00(deflate) 解压失败: %v\n", err)
				continue
			}
			fmt.Printf("    压缩头=0x00(deflate) 解压 %d -> %d 字节\n", len(body)-1, len(out))
			body = out
		case 0xFF:
			fmt.Printf("    压缩头=0xFF(未压缩) %d 字节\n", len(body)-1)
			body = body[1:]
		default:
			fmt.Printf("    无压缩头（明文批次）%d 字节\n", len(body))
		}
		dumpFrames(body)
	}
}

// inflate 解 raw deflate（无 zlib 头）。
func inflate(b []byte) ([]byte, error) {
	r := flate.NewReader(bytes.NewReader(b))
	defer r.Close()
	return io.ReadAll(r)
}

// dumpFrames 解析 [varint frameLen][varint header][payload] 结构。
func dumpFrames(b []byte) {
	i := 0
	for i < len(b) {
		frameLen, n := readVarint(b[i:])
		i += n
		if frameLen == 0 {
			fmt.Printf("    帧长=0，停止（剩余 %d 字节）\n", len(b)-i)
			return
		}
		if i+int(frameLen) > len(b) {
			fmt.Printf("    帧长=%d 越界（剩余 %d）\n", frameLen, len(b)-i)
			return
		}
		frame := b[i : i+int(frameLen)]
		i += int(frameLen)

		id, n := readVarint(frame)
		payload := frame[n:]
		show := payload
		if len(show) > 80 {
			show = show[:80]
		}
		fmt.Printf("    包: 帧长=%d id=%d(0x%x) 负载=%d 字节 hex=%x", frameLen, id&0x3ff, id, len(payload), show)
		if len(payload) > 80 {
			fmt.Printf("...")
		}
		if isPrintable(payload) && len(payload) > 0 {
			fmt.Printf(" ascii=%q", string(payload))
		}
		fmt.Println()
	}
}

func readVarint(b []byte) (uint32, int) {
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

func isPrintable(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	n := 0
	for _, c := range b {
		if c >= 0x20 && c < 0x7f {
			n++
		}
	}
	return n*10 >= len(b)*8 && len(b) > 3
}

var _ = strconv.Itoa
