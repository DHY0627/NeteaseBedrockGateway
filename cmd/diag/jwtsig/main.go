// jwtsig 验证握手 JWT 的 ES384 签名是否与 x5u 公钥匹配。
package main

import (
	"crypto/ecdsa"
	"crypto/sha512"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"strings"
)

func main() {
	jwt := os.Args[1]
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		fmt.Println("JWT 段数异常:", len(parts))
		return
	}
	hdrBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		fmt.Println("header 解码失败:", err)
		return
	}
	var hdr map[string]interface{}
	json.Unmarshal(hdrBytes, &hdr)
	fmt.Printf("header: %s\n", hdrBytes)
	payloadBytes, _ := base64.RawURLEncoding.DecodeString(parts[1])
	fmt.Printf("payload: %s\n", payloadBytes)

	x5u, _ := hdr["x5u"].(string)
	keyDER, err := base64.StdEncoding.DecodeString(x5u)
	if err != nil {
		keyDER, err = base64.RawURLEncoding.DecodeString(x5u)
		if err != nil {
			fmt.Println("x5u base64 解码失败:", err)
			return
		}
	}
	fmt.Printf("x5u DER (%d 字节): %x\n", len(keyDER), keyDER[:min(24, len(keyDER))])

	pubAny, err := x509.ParsePKIXPublicKey(keyDER)
	if err != nil {
		fmt.Println("x5u 不是 X509 公钥:", err)
		fmt.Printf("尝试作为证书解析...\n")
		if cert, cerr := x509.ParseCertificate(keyDER); cerr == nil {
			fmt.Printf("是证书! Subject=%s\n", cert.Subject)
			pubAny = cert.PublicKey
		} else {
			fmt.Println("证书解析也失败:", cerr)
			return
		}
	}
	pub, ok := pubAny.(*ecdsa.PublicKey)
	if !ok {
		fmt.Println("不是 EC 公钥")
		return
	}
	fmt.Printf("曲线: %s (位数 %d)\n", pub.Curve.Params().Name, pub.Curve.Params().BitSize)

	sigBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		fmt.Println("签名解码失败:", err)
		return
	}
	fmt.Printf("签名 %d 字节\n", len(sigBytes))

	signingInput := parts[0] + "." + parts[1]
	digest := sha512.Sum384([]byte(signingInput))

	// JWS 格式：r||s，各 48 字节
	if len(sigBytes) == 96 {
		r := new(big.Int).SetBytes(sigBytes[:48])
		s := new(big.Int).SetBytes(sigBytes[48:])
		fmt.Printf("JWS(r||s) 验证: %v\n", ecdsa.Verify(pub, digest[:], r, s))
	}
	// ASN.1 DER 格式
	var rs struct{ R, S *big.Int }
	if _, err := asn1.Unmarshal(sigBytes, &rs); err == nil && rs.R != nil {
		fmt.Printf("DER 验证: %v\n", ecdsa.Verify(pub, digest[:], rs.R, rs.S))
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
