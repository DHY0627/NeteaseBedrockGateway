package utils

// 实现 pyaes.AESModeOfOperationECB

import (
	"bytes"
	"crypto/aes"
	"fmt"
)

// pkcs7Padding 为数据添加 PKCS#7 填充（使长度为 16 字节整数倍）
func pkcs7Padding(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	padText := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(data, padText...)
}

// AesECBEncrypt AES-ECB 加密（对应 pyaes.AESModeOfOperationECB.encrypt）
func AesECBEncrypt(plaintext, key []byte) ([]byte, error) {
	// 1. 初始化 AES 密码器（对应 Python 中 _aes 对象）
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	blockSize := block.BlockSize() // 固定为 16 字节（AES 块大小）
	// 2. 填充数据（确保为 16 字节整数倍，对应 Python 中对 16 字节块的要求）
	plaintext = pkcs7Padding(plaintext, blockSize)
	ciphertext := make([]byte, len(plaintext))

	// 3. 逐块加密（ECB 核心逻辑：每个块独立处理）
	for i := 0; i < len(plaintext); i += blockSize {
		block.Encrypt(ciphertext[i:i+blockSize], plaintext[i:i+blockSize])
	}
	return ciphertext, nil
}

// AesECBEncryptBlock 执行无填充的 AES-ECB 加密，要求明文长度为 16 的倍数。
func AesECBEncryptBlock(plaintext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(plaintext)%block.BlockSize() != 0 {
		return nil, fmt.Errorf("aes ecb encrypt: plaintext length %d is not block aligned", len(plaintext))
	}

	ciphertext := make([]byte, len(plaintext))
	for i := 0; i < len(plaintext); i += block.BlockSize() {
		block.Encrypt(ciphertext[i:i+block.BlockSize()], plaintext[i:i+block.BlockSize()])
	}
	return ciphertext, nil
}
