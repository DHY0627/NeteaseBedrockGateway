// dectest 尝试多种解压方式解析响应。
package main

import (
	"bytes"
	"compress/flate"
	"compress/zlib"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
)

func main() {
	raw, _ := os.ReadFile(os.Args[1])
	h := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F' {
			return r
		}
		return -1
	}, string(raw))
	if len(h)%2 != 0 {
		h = h[:len(h)-1]
	}
	b, _ := hex.DecodeString(h)
	fmt.Printf("输入 %d 字节: %x\n", len(b), b)

	try := func(name string, data []byte, mk func(io.Reader) (io.ReadCloser, error)) {
		r, err := mk(bytes.NewReader(data))
		if err != nil {
			fmt.Printf("%s: 构造失败 %v\n", name, err)
			return
		}
		out, err := io.ReadAll(r)
		fmt.Printf("%s: %d 字节 (err=%v)\n  hex: %x\n  文本: %s\n", name, len(out), err, out[:min(200, len(out))], printable(out))
	}
	// 跳过 FE
	if len(b) > 0 && b[0] == 0xFE {
		b = b[1:]
	}
	try("zlib(跳过FE)", b, func(r io.Reader) (io.ReadCloser, error) { return zlib.NewReader(r) })
	try("raw-deflate(跳过FE)", b, func(r io.Reader) (io.ReadCloser, error) { return flate.NewReader(r), nil })
	if len(b) > 1 {
		try("raw-deflate(跳FE+1)", b[1:], func(r io.Reader) (io.ReadCloser, error) { return flate.NewReader(r), nil })
		try("zlib(跳FE+1)", b[1:], func(r io.Reader) (io.ReadCloser, error) { return zlib.NewReader(r) })
	}
}

func printable(b []byte) string {
	var sb strings.Builder
	for _, c := range b {
		if c >= 0x20 && c < 0x7f {
			sb.WriteByte(c)
		} else {
			sb.WriteByte('.')
		}
	}
	return sb.String()
}
