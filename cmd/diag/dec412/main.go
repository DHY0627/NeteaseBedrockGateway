// dec412 尝试解密真实客户端的 TanCreateRoomRequest（帧 #1915）。
// 密钥 = MD5(userToken) + raknetRand；AESRand = AES_ECB(Rand, encryptedToken)。
// 通过验证 AESRand 来确认 userToken 候选，然后解密 #412 密文。
package main

import (
	"crypto/aes"
	"crypto/md5"
	"encoding/hex"
	"fmt"
)

func aesECBEncrypt(plain, key []byte) []byte {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil
	}
	out := make([]byte, len(plain))
	for i := 0; i < len(plain); i += 16 {
		block.Encrypt(out[i:i+16], plain[i:i+16])
	}
	return out
}

func main() {
	randHex := "e112a85f1d1f9be244c247c60da198c1"
	aesRandHex := "0abf98df762ae3f5841a38972f1e7fb7"
	randBytes, _ := hex.DecodeString(randHex)
	_, _ = hex.DecodeString(aesRandHex)

	// 密文 #412（去掉 fee301）
	cipherHex := "ed9fabd9dbecef5352c37318e0a65fc829deb925ae440754a2bb44b1fa15511b00f3c268b1cc46742a76d2ba792a3156734e463fc5e3feb10a35a1bf9e523aa06e21d3512e83dcbf723fce44eb0a401691b8733ecaa666555595de803cf3f1592caa65884e"
	_, _ = hex.DecodeString(cipherHex)

	// 尝试 userToken 候选
	candidates := []string{
		"178809667",
		"3063529045",
		"178809667:3063529045",
		"3063529045:178809667",
	}

	for _, tok := range candidates {
		md := md5.Sum([]byte(tok))
		encTok := md[:]
		computed := aesECBEncrypt(randBytes, encTok)
		match := hex.EncodeToString(computed) == aesRandHex
		fmt.Printf("userToken=%q encryptedToken=%x AESRand匹配=%v\n", tok, encTok, match)
		if match {
			fmt.Println("!!! 找到 encryptedToken，尝试解密 #412")
			// 这里需要 chacha8 解密，先跳过
		}
	}

	fmt.Println("\n未匹配——userToken 不是简单候选。可能需要从客户端获取完整 userToken。")
}
