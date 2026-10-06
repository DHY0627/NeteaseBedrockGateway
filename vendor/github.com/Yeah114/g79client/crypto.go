package g79client

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

// AES密钥列表
var keys = []string{
	"60F1E0D1FD635362430747215CF1C2FF",
	"EA5B62D27D0338374852C4B9469D7AC6",
	"17238D55501C5F020B155FB3303591E6",
	"8C5CEAE0F443E006A050266F73ADD5B0",
	"1C02CE22FB22F0E72060217418F351F3",
	"9A01773FEBB0CFE0EBDBF37F4D23C27F",
	"43F32300BF252CC320E2572ACE766367",
	"07F161011B3101F1ED0301735631E734",
	"0454E7707A5F37565601E100406060AF",
	"647554BAD3100C43C16660F002CC10F3",
	"E157213170F842382032564265B0B043",
	"914FC59311B04151393EF6896A847636",
	"0710C0205D224237025323265C145FA1",
	"054E6F01165267025C3111F562A921E9",
	"722D1789E792E2CA0D5322211FD0F5AE",
	"91F7C751FCF671F34943430772341799",
}

var x19Keys = []string{
	"MK6mipwmOUedplb6",
	"OtEylfId6dyhrfdn",
	"VNbhn5mvUaQaeOo9",
	"bIEoQGQYjKd02U0J",
	"fuaJrPwaH2cfXXLP",
	"LEkdyiroouKQ4XN1",
	"jM1h27H4UROu427W",
	"DhReQada7gZybTDk",
	"ZGXfpSTYUvcdKqdY",
	"AZwKf7MWZrJpGR5W",
	"amuvbcHw38TcSyPU",
	"SI4QotspbjhyFdT0",
	"VP4dhjKnDGlSJtbB",
	"UXDZx4KhZywQ2tcn",
	"NIK73ZNvNqzva4kd",
	"WeiW7qU766Q1YQZI",
}

// 生成随机字符串
func randomString(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)
	for i := range result {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		result[i] = charset[num.Int64()]
	}
	return string(result), nil
}

func randomBytes(length int) ([]byte, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}

// HTTP加密
func G79HttpEncrypt(body []byte) ([]byte, error) {
	// 对齐到16字节块并添加16个随机字节
	blockSize := 16
	targetLen := ((len(body) + blockSize + blockSize - 1) / blockSize) * blockSize
	buffer := make([]byte, targetLen)
	copy(buffer, body)

	// 添加16个随机字节
	tailRandom, err := randomString(16)
	if err != nil {
		return nil, err
	}
	copy(buffer[len(body):], []byte(tailRandom))

	// 生成标志位
	highNibble, err := rand.Int(rand.Reader, big.NewInt(15))
	if err != nil {
		return nil, err
	}
	flag := byte((highNibble.Int64() << 4) | 0x0C)

	// 生成IV
	iv, err := randomString(16)
	if err != nil {
		return nil, err
	}
	ivBytes := []byte(iv)

	// 选择密钥
	keyIndex := (flag >> 4) & 0x0F
	keyBytes, err := hex.DecodeString(keys[keyIndex])
	if err != nil {
		return nil, err
	}

	// AES加密
	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return nil, err
	}

	mode := cipher.NewCBCEncrypter(block, ivBytes)
	cipherText := make([]byte, len(buffer))
	mode.CryptBlocks(cipherText, buffer)

	// 组装结果
	result := make([]byte, 16+len(cipherText)+1)
	copy(result[0:16], ivBytes)
	copy(result[16:16+len(cipherText)], cipherText)
	result[len(result)-1] = flag

	return result, nil
}

// HTTP解密
func G79HttpDecrypt(payload []byte) ([]byte, error) {
	if len(payload) < 18 {
		return nil, fmt.Errorf("payload too short")
	}

	flag := payload[len(payload)-1]
	iv := payload[0:16]
	cipherText := payload[16 : len(payload)-1]

	keyIndex := (flag >> 4) & 0x0F
	keyBytes, err := hex.DecodeString(keys[keyIndex])
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return nil, err
	}

	mode := cipher.NewCBCDecrypter(block, iv)
	plainText := make([]byte, len(cipherText))
	mode.CryptBlocks(plainText, cipherText)

	// 移除尾部的零字节
	lastNonZero := len(plainText) - 1
	for lastNonZero >= 0 && plainText[lastNonZero] == 0 {
		lastNonZero--
	}
	if lastNonZero < 0 {
		return []byte{}, nil
	}

	return plainText[:lastNonZero+1], nil
}

// 计算动态token
func CalculateDynamicToken(path, content, token string) string {
	salt := "0eGsBkhl"

	// 计算token的MD5
	tokenMd5 := fmt.Sprintf("%x", md5.Sum([]byte(token)))

	// 构建payload
	payload := tokenMd5 + content + salt + strings.TrimSuffix(path, "?")

	// 计算payload的MD5
	payloadMd5 := fmt.Sprintf("%x", md5.Sum([]byte(payload)))

	// 转换为二进制字符串
	binaryString := ""
	for _, b := range []byte(payloadMd5) {
		binaryString += fmt.Sprintf("%08b", b)
	}

	// 左移6位
	binaryString = binaryString[6:] + binaryString[:6]

	// 对每个字节进行位反转并异或
	transformed := make([]byte, len(payloadMd5))
	copy(transformed, []byte(payloadMd5))

	for i := 0; i < len(transformed); i++ {
		section := binaryString[i*8 : i*8+8]
		reversedByte := byte(0)
		for j := 0; j < 8; j++ {
			if section[7-j] == '1' {
				reversedByte |= (1 << (j & 0x1F))
			}
		}
		transformed[i] = reversedByte ^ transformed[i]
	}

	// Base64编码并处理
	b64 := base64.StdEncoding.EncodeToString(transformed)
	tokenShort := strings.ReplaceAll(strings.ReplaceAll(b64[:16], "+", "m"), "/", "o") + "1"

	return tokenShort
}

// X19HttpEncrypt implements the PC launcher encryption used by the X19 endpoints.
func X19HttpEncrypt(body []byte) ([]byte, error) {
	content := make([]byte, len(body))
	copy(content, body)

	tail, err := randomBytes(16)
	if err != nil {
		return nil, err
	}
	content = append(content, tail...)

	keyIndexBig, err := rand.Int(rand.Reader, big.NewInt(int64(len(x19Keys))))
	if err != nil {
		return nil, err
	}
	keyIndex := int(keyIndexBig.Int64())

	iv, err := randomBytes(16)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher([]byte(x19Keys[keyIndex]))
	if err != nil {
		return nil, err
	}

	padded := pkcs7Pad(content, aes.BlockSize)
	encrypted := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(encrypted, padded)

	result := make([]byte, 0, len(iv)+len(encrypted)+1)
	result = append(result, iv...)
	result = append(result, encrypted...)
	result = append(result, byte(keyIndex<<4)|0x02)
	return result, nil
}

// X19HttpDecrypt decrypts payloads produced by X19HttpEncrypt.
func X19HttpDecrypt(payload []byte) ([]byte, error) {
	if len(payload) < 18 {
		return nil, fmt.Errorf("payload too short")
	}

	flag := payload[len(payload)-1]
	keyIndex := int((flag >> 4) & 0x0F)
	if keyIndex >= len(x19Keys) {
		return nil, fmt.Errorf("invalid key index %d", keyIndex)
	}

	iv := payload[:16]
	cipherText := payload[16 : len(payload)-1]

	block, err := aes.NewCipher([]byte(x19Keys[keyIndex]))
	if err != nil {
		return nil, err
	}

	plain := make([]byte, len(cipherText))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plain, cipherText)
	return plain, nil
}

// X19CalculateDynamicToken reproduces the PC launcher token logic.
func X19CalculateDynamicToken(url, content, token string) (string, error) {
	tokenMD5 := fmt.Sprintf("%x", md5.Sum([]byte(token)))
	magicMD5 := fmt.Sprintf("%x", md5.Sum([]byte(tokenMD5+content+"0eGsBkhl"+url)))

	binaryMagic := stringToBin(magicMD5)
	shifted := stringLeftShift(binaryMagic, 6)
	shiftedStr, err := binToString(shifted)
	if err != nil {
		return "", err
	}

	xorBytes, err := stringXOR(magicMD5, shiftedStr)
	if err != nil {
		return "", err
	}

	encoded := base64.StdEncoding.EncodeToString(xorBytes)
	replaced := strings.ReplaceAll(strings.ReplaceAll(encoded, "/", "o"), "+", "m")
	if len(replaced) < 16 {
		return "", fmt.Errorf("token too short")
	}
	return replaced[:16] + "1", nil
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	padLen := blockSize - len(data)%blockSize
	if padLen == 0 {
		padLen = blockSize
	}
	padding := bytesRepeat(byte(padLen), padLen)
	return append(data, padding...)
}

func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 || len(data)%blockSize != 0 {
		return nil, fmt.Errorf("invalid padding")
	}
	padLen := int(data[len(data)-1])
	if padLen == 0 || padLen > blockSize || padLen > len(data) {
		return nil, fmt.Errorf("invalid padding size")
	}
	for _, b := range data[len(data)-padLen:] {
		if int(b) != padLen {
			return nil, fmt.Errorf("invalid padding content")
		}
	}
	return data[:len(data)-padLen], nil
}

func bytesRepeat(b byte, count int) []byte {
	res := make([]byte, count)
	for i := range res {
		res[i] = b
	}
	return res
}

func stringToBin(s string) string {
	var builder strings.Builder
	for i := 0; i < len(s); i++ {
		builder.WriteString(fmt.Sprintf("%08b", s[i]))
	}
	return builder.String()
}

func stringLeftShift(s string, n int) string {
	if len(s) == 0 {
		return s
	}
	n = n % len(s)
	return s[n:] + s[:n]
}

func binToString(bin string) (string, error) {
	if len(bin)%8 != 0 {
		return "", fmt.Errorf("binary length must be multiple of 8")
	}
	result := make([]byte, len(bin)/8)
	for i := 0; i < len(result); i++ {
		chunk := bin[i*8 : (i+1)*8]
		value, err := strconv.ParseUint(chunk, 2, 8)
		if err != nil {
			return "", err
		}
		result[i] = byte(value)
	}
	return string(result), nil
}

func stringXOR(a, b string) ([]byte, error) {
	if len(a) != len(b) {
		return nil, fmt.Errorf("length mismatch")
	}
	out := make([]byte, len(a))
	for i := range a {
		out[i] = a[i] ^ b[i]
	}
	return out, nil
}

// 获取有效JSON边界
func GetValidJSON(decryptedBody []byte) []byte {
	blocksCount := 0
	readableLength := 0

	for i, b := range decryptedBody {
		switch b {
		case 0x7B: // '{'
			blocksCount++
		case 0x7D: // '}'
			blocksCount--
		}

		if blocksCount == 0 && i != 0 {
			readableLength = i + 1
			break
		}
	}

	if readableLength != 0 {
		return decryptedBody[:readableLength]
	}
	return decryptedBody
}
