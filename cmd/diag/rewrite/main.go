// rewrite 把捕获的 Login 链里的 netease_uid/identity 改写成另一个账号，
// 用于幽灵玩家加入"房主是自己账号"的房间时绕过 loggedinOtherLocation 拒绝。
package main

import (
	"bytes"
	"compress/flate"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

func main() {
	if len(os.Args) < 5 {
		fmt.Println("usage: rewrite <in-hex> <out-hex> <new-uid> <new-identity-uuid> [new-name]")
		return
	}
	inHex, _ := os.ReadFile(os.Args[1])
	outPath := os.Args[2]
	newUID := os.Args[3]
	newIdentity := os.Args[4]
	newName := ""
	if len(os.Args) > 5 {
		newName = os.Args[5]
	}

	h := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F' {
			return r
		}
		return -1
	}, string(inHex))
	if len(h)%2 != 0 {
		h = h[:len(h)-1]
	}
	comp, _ := hex.DecodeString(h)

	// 解压（无前缀 deflate 或 [00][deflate]）
	start := 0
	if comp[0] == 0x00 {
		start = 1
	}
	raw, err := io.ReadAll(flate.NewReader(bytes.NewReader(comp[start:])))
	if err != nil && len(raw) == 0 {
		fmt.Println("解压失败:", err)
		return
	}
	fmt.Printf("解压 %d -> %d 字节\n", len(comp)-start, len(raw))

	// 帧: [varint frameLen][header][payload]
	frameLen, n1 := readVarInt(raw)
	frame := raw[n1 : n1+int(frameLen)]
	header, n2 := readVarInt(frame)
	payload := frame[n2:]
	if header&0x3ff != 1 {
		fmt.Printf("非 Login 包 (header=%d)\n", header)
		return
	}

	// payload: [int32 version][varint restLen][uint32 LE jsonLen][JSON][skin...]
	version := payload[0:4]
	restLen, n3 := readVarInt(payload[4:])
	rest := payload[4+n3:]
	fmt.Printf("version=%d restLen=%d 实际剩余=%d\n", beUint32(version), restLen, len(rest))
	body := rest[4:]

	// 找 JSON 结尾：逐个长度尝试解析
	var loginJSON map[string]interface{}
	jsonLen := 0
	for end := len(body); end > 0; end-- {
		if json.Unmarshal(body[:end], &loginJSON) == nil {
			jsonLen = end
			break
		}
	}
	if loginJSON == nil {
		fmt.Println("JSON 解析失败")
		return
	}
	jsonLenField := uint32(rest[0]) | uint32(rest[1])<<8 | uint32(rest[2])<<16 | uint32(rest[3])<<24
	fmt.Printf("jsonLen 字段=%d 实际=%d %v\n", jsonLenField, jsonLen, map[bool]string{true: "✓一致", false: "✗不一致"}[int(jsonLenField) == jsonLen])
	skin := body[jsonLen:]
	fmt.Printf("JSON=%d 字节, 尾部(skin)=%d 字节\n", jsonLen, len(skin))

	// 改写链
	certStr, _ := loginJSON["Certificate"].(string)
	var certObj map[string]interface{}
	json.Unmarshal([]byte(certStr), &certObj)
	chain, _ := certObj["chain"].([]interface{})
	if len(chain) == 0 {
		fmt.Println("链为空")
		return
	}
	jwt := chain[0].(string)
	parts := strings.SplitN(jwt, ".", 3)
	if len(parts) != 3 {
		fmt.Println("JWT 段数异常")
		return
	}
	payloadJSON, _ := base64.RawURLEncoding.DecodeString(strings.TrimSpace(parts[1]))
	var claims map[string]interface{}
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		fmt.Println("claims 解析失败:", err)
		return
	}
	if extra, ok := claims["extraData"].(map[string]interface{}); ok {
		fmt.Printf("原 uid=%v identity=%v name=%v\n", extra["netease_uid"], extra["identity"], extra["displayName"])
		extra["netease_uid"] = newUID
		extra["identity"] = newIdentity
		if newName != "" {
			extra["displayName"] = newName
		}
	}
	newPayload, _ := json.Marshal(claims)
	newJWT := strings.TrimSpace(parts[0]) + "." + base64.RawURLEncoding.EncodeToString(newPayload) + "." + strings.TrimSpace(parts[2])

	// 重建 Certificate 与最外层 JSON
	certObj["chain"] = []interface{}{newJWT}
	newCert, _ := json.Marshal(certObj)
	loginJSON["Certificate"] = string(newCert)

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.Encode(loginJSON)
	newJSON := strings.TrimRight(buf.String(), "\n")
	fmt.Printf("新 JSON=%d 字节 (原 %d)\n", len(newJSON), jsonLen)

	// 重建 payload / frame
	newJSONB := []byte(newJSON)
	newBody := make([]byte, 4)
	newBody[0] = byte(len(newJSONB))
	newBody[1] = byte(len(newJSONB) >> 8)
	newBody[2] = byte(len(newJSONB) >> 16)
	newBody[3] = byte(len(newJSONB) >> 24)
	newBody = append(newBody, newJSONB...)
	newBody = append(newBody, skin...)
	newRest := newBody
	var restBuf bytes.Buffer
	writeVarInt(&restBuf, uint32(len(newRest)))
	framePayload := append([]byte{}, version...)
	framePayload = append(framePayload, restBuf.Bytes()...)
	framePayload = append(framePayload, newRest...)

	var newFrame bytes.Buffer
	writeVarInt(&newFrame, uint32(len(framePayload)+1))
	writeVarInt(&newFrame, header)
	newFrame.Write(framePayload)

	// 压缩为 raw deflate（无前缀，客户端对 NONE 房主的格式）
	var outBuf bytes.Buffer
	w, _ := flate.NewWriter(&outBuf, flate.DefaultCompression)
	w.Write(newFrame.Bytes())
	w.Close()

	outHex := hex.EncodeToString(outBuf.Bytes())
	_ = restLen
	os.WriteFile(outPath, []byte(outHex), 0644)
	fmt.Printf("已写出 %s (%d 字节压缩, %d hex)\n", outPath, outBuf.Len(), len(outHex))
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
