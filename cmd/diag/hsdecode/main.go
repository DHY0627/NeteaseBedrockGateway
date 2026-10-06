// hsdecode 解压握手消息并输出完整 JWT。
package main

import (
	"bytes"
	"compress/flate"
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
	// [FE][00][deflate]
	start := 0
	if len(b) > 0 && b[0] == 0xFE {
		start = 1
	}
	if len(b) > start && b[start] == 0x00 {
		start++
	}
	out, err := io.ReadAll(flate.NewReader(bytes.NewReader(b[start:])))
	fmt.Printf("解压 %d -> %d 字节 (err=%v)\n", len(b)-start, len(out), err)

	// frame: [varint frameLen][varint header][varint strLen][JWT]
	fl, n1 := readVarInt(out)
	fmt.Printf("frameLen=%d\n", fl)
	frame := out[n1:]
	hd, n2 := readVarInt(frame)
	fmt.Printf("header=%d packetId=%d\n", hd, hd&0x3ff)
	sl, n3 := readVarInt(frame[n2:])
	fmt.Printf("字符串长度=%d\n", sl)
	jwt := string(frame[n2+n3 : n2+n3+int(sl)])
	fmt.Printf("JWT 长度=%d\n", len(jwt))
	fmt.Println(jwt)
	parts := strings.Split(jwt, ".")
	for i, p := range parts {
		fmt.Printf("  段%d: %d 字符 -> %d 字节\n", i, len(p), len(p)*3/4)
	}
}

func readVarInt(b []byte) (uint32, int) {
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
