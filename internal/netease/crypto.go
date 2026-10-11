package netease

import (
	"bytes"
	"crypto/aes"
	"crypto/md5"
	"encoding/hex"
)

// md5hex 返回字符串的 MD5 十六进制小写表示。
// mpay 的登录参数使用 md5(密码) 而不是明文密码。
func md5hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

// aesECBPKCS7Hex 复刻官方启动器 MpayLoginEncrypt：AES/ECB/PKCS7Padding 后输出 hex 小写字符串。
// key 为 16 字节的 AES-128 密钥（由设备密钥 hex 解码得到）。
func aesECBPKCS7Hex(plain []byte, key []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	padLen := aes.BlockSize - len(plain)%aes.BlockSize
	padded := append(append([]byte{}, plain...), bytes.Repeat([]byte{byte(padLen)}, padLen)...)

	out := make([]byte, len(padded))
	for i := 0; i < len(padded); i += aes.BlockSize {
		block.Encrypt(out[i:i+aes.BlockSize], padded[i:i+aes.BlockSize])
	}
	return hex.EncodeToString(out), nil
}

// aesECBPKCS7Decrypt 是 aesECBPKCS7Hex 的逆运算，仅用于测试与自检。
func aesECBPKCS7Decrypt(cipherHex string, key []byte) ([]byte, error) {
	raw, err := hex.DecodeString(cipherHex)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 || len(raw)%aes.BlockSize != 0 {
		return nil, errInvalidCipherText
	}
	out := make([]byte, len(raw))
	for i := 0; i < len(raw); i += aes.BlockSize {
		block.Decrypt(out[i:i+aes.BlockSize], raw[i:i+aes.BlockSize])
	}
	if len(out) == 0 {
		return nil, errInvalidCipherText
	}
	pad := int(out[len(out)-1])
	if pad <= 0 || pad > aes.BlockSize || pad > len(out) {
		return nil, errInvalidCipherText
	}
	return out[:len(out)-pad], nil
}

type constError string

func (e constError) Error() string { return string(e) }

const errInvalidCipherText = constError("密文长度或填充非法")
