// verifyrebuild 校验 Login 解析→重建 是否与原始字节完全一致。
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
	comp, _ := hex.DecodeString(h)
	start := 0
	if comp[0] == 0x00 {
		start = 1
	}
	orig, _ := io.ReadAll(flate.NewReader(bytes.NewReader(comp[start:])))
	fmt.Printf("原始解压: %d 字节\n", len(orig))

	// 解析: [varint frameLen][varint header][int32 version][varint restLen][uint32 jsonLen][JSON][uint32 skinLen][skin]
	frameLen, n1 := readVarInt(orig)
	frame := orig[n1 : n1+int(frameLen)]
	header, n2 := readVarInt(frame)
	payload := frame[n2:]
	version := payload[0:4]
	_, n3 := readVarInt(payload[4:])
	rest := payload[4+n3:]
	jsonLenField := uint32(rest[0]) | uint32(rest[1])<<8 | uint32(rest[2])<<16 | uint32(rest[3])<<24
	body := rest[4:]
	jsonBytes := body[:jsonLenField]
	afterJSON := body[jsonLenField:]
	skinLenField := uint32(afterJSON[0]) | uint32(afterJSON[1])<<8 | uint32(afterJSON[2])<<16 | uint32(afterJSON[3])<<24
	skinBytes := afterJSON[4:]
	fmt.Printf("frameLen=%d header=%d version=%d restLen=%d jsonLen(字段)=%d JSON实际长度=%d afterJSON=%d skinLen(字段)=%d skin实际=%d\n",
		frameLen, header, beUint32(version), n3, jsonLenField, len(jsonBytes), len(afterJSON), skinLenField, len(skinBytes))

	// 重建
	var out bytes.Buffer
	body2 := make([]byte, 4)
	body2[0] = byte(len(jsonBytes))
	body2[1] = byte(len(jsonBytes) >> 8)
	body2[2] = byte(len(jsonBytes) >> 16)
	body2[3] = byte(len(jsonBytes) >> 24)
	body2 = append(body2, jsonBytes...)
	body2 = append(body2, afterJSON...) // 含 skinLen 字段 + skin
	var restBuf bytes.Buffer
	writeVarInt(&restBuf, uint32(len(body2)))
	fp := append([]byte{}, version...)
	fp = append(fp, restBuf.Bytes()...)
	fp = append(fp, body2...)
	writeVarInt(&out, uint32(len(fp)+1))
	writeVarInt(&out, header)
	out.Write(fp)

	rebuilt := out.Bytes()
	fmt.Printf("重建长度=%d 原始长度=%d 一致=%v\n", len(rebuilt), len(orig), bytes.Equal(rebuilt, orig))
	if !bytes.Equal(rebuilt, orig) {
		for i := 0; i < len(orig) && i < len(rebuilt); i++ {
			if orig[i] != rebuilt[i] {
				fmt.Printf("首个差异 @ %d: 原始=%02x 重建=%02x\n", i, orig[i], rebuilt[i])
				fmt.Printf("原始上下文: %x\n", orig[max(0, i-16):min(len(orig), i+16)])
				fmt.Printf("重建上下文: %x\n", rebuilt[max(0, i-16):min(len(rebuilt), i+16)])
				break
			}
		}
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

func writeVarInt(b *bytes.Buffer, v uint32) {
	for {
		c := byte(v & 0x7f)
		v >>= 7
		if v != 0 {
			b.WriteByte(c | 0x80)
		} else {
			b.WriteByte(c)
			return
		}
	}
}

func beUint32(b []byte) uint32 {
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}
