package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"
)

// 初始化随机数生成器（用于生成随机字符串）
func init() {
	rand.Seed(time.Now().UnixNano())
}

// GetEncryptedToken 对登录令牌做 MD5 哈希并返回字节切片
func GetEncryptedToken(loginToken string) []byte {
	h := md5.New()
	h.Write([]byte(loginToken))
	return h.Sum(nil)
}

// G79GetDynamicHttpToken 生成 G79 动态 HTTP 令牌
func G79GetDynamicHttpToken(loginToken, path, body string) (string, error) {
	// 字符串转二进制字符串（每个字符8位补零）
	stringToBin := func(s string) string {
		var binStr strings.Builder
		for _, c := range s {
			fmt.Fprintf(&binStr, "%08b", byte(c))
		}
		return binStr.String()
	}

	// 字符串左移（超出长度取模）
	stringLeftShift := func(s string, n int) string {
		lenS := len(s)
		if lenS == 0 {
			return s
		}
		n = n % lenS
		return s[n:] + s[:n]
	}

	// 二进制字符串转字节切片
	binToBytes := func(binary string) ([]byte, error) {
		lenBin := len(binary)
		if lenBin%8 != 0 {
			return nil, fmt.Errorf("binary length %d not multiple of 8", lenBin)
		}

		var bytes []byte
		for i := 0; i < lenBin; i += 8 {
			chunk := binary[i : i+8]
			b, err := strconv.ParseUint(chunk, 2, 8)
			if err != nil {
				return nil, fmt.Errorf("parse binary chunk %q: %w", chunk, err)
			}
			bytes = append(bytes, uint8(b))
		}
		return bytes, nil
	}

	// 字节切片按位异或
	bytesXor := func(s, key []byte) ([]byte, error) {
		if len(s) != len(key) {
			return nil, fmt.Errorf("length mismatch: s=%d, key=%d", len(s), len(key))
		}

		xor := make([]byte, len(s))
		for i := range s {
			xor[i] = s[i] ^ key[i]
		}
		return xor, nil
	}

	// 令牌最终加密处理
	encryptToken := func(s string) string {
		replaced := strings.ReplaceAll(strings.ReplaceAll(s, "+", "m"), "/", "o")
		prefix := replaced
		if len(replaced) >= 16 {
			prefix = replaced[:16]
		}
		return prefix + "1"
	}

	// 步骤1：MD5 哈希计算
	h := md5.New()
	if _, err := h.Write([]byte(loginToken)); err != nil {
		return "", fmt.Errorf("write login token: %w", err)
	}
	loginTokenHash := h.Sum(nil)
	h.Reset()

	// 步骤2：更新哈希内容
	hexLoginHash := hex.EncodeToString(loginTokenHash)
	if _, err := h.Write([]byte(hexLoginHash)); err != nil {
		return "", fmt.Errorf("write hex login hash: %w", err)
	}
	if _, err := h.Write([]byte(body)); err != nil {
		return "", fmt.Errorf("write body: %w", err)
	}
	pathStr := fmt.Sprintf("0eGsBkhl%s", path)
	if _, err := h.Write([]byte(pathStr)); err != nil {
		return "", fmt.Errorf("write path string: %w", err)
	}

	// 步骤3：二进制转换与移位
	string2 := hex.EncodeToString(h.Sum(nil))
	string3 := stringToBin(string2)
	string4 := stringLeftShift(string3, 6)

	// 步骤4：异或与 Base64 编码
	string5Bytes, err := binToBytes(string4)
	if err != nil {
		return "", fmt.Errorf("bin to bytes: %w", err)
	}
	string6Bytes, err := bytesXor([]byte(string2), string5Bytes)
	if err != nil {
		return "", fmt.Errorf("bytes xor: %w", err)
	}
	string7 := base64.StdEncoding.EncodeToString(string6Bytes)

	return encryptToken(string7), nil
}

// G79PickKey 根据索引选择并处理密钥（十六进制解码 + 异或 0x7C）
func G79PickKey(index uint8) ([]byte, error) {
	keys := []string{
		"1C8D9CAD811F2F1E3F7B3B5D208DBE83",
		"96271EAE017F444B342EB8C53AE106BA",
		"6B5FF1292C60237E776923CF4C49ED9A",
		"F020969C883F9C7ADC2C5A130FD1A9CC",
		"607EB25E875E8C9B5C1C5D08648F2D8F",
		"E67D0B4397CCB39C97A78F03315FBE03",
		"3F8F5F7CC35950BF5C9E2B56B20A1F1B",
		"7B8D1D7D674D7D8D917F7D0F2A4D9B48",
		"78289B0C06234B2A2A7D9D7C3C1C1CD3",
		"180928C6AF6C703FBD1A1C8C7EB06C8F",
		"9D2B5D4D0C843E445C4E2A3E19CCCC3F",
		"ED33B9EF6DCC3D2D45428AF516F80A4A",
		"7B6CBC5C215E3E4B7E2F5F5A206823DD",
		"7932137D6A2E1B7E204D6D891ED55D95",
		"0E516BF59BEE9EB6712F5E5D63AC89D2",
		"ED8BBB2D808A0D8F353F3F7B0E486BE5",
	}

	// 检查索引有效性
	if index >= uint8(len(keys)) {
		return nil, fmt.Errorf("index %d out of range (0-%d)", index, len(keys)-1)
	}

	// 十六进制解码
	hexKey, err := hex.DecodeString(keys[index])
	if err != nil {
		return nil, fmt.Errorf("decode key hex: %w", err)
	}

	// 每个字节异或 0x7C
	for i := range hexKey {
		hexKey[i] ^= 0x7C
	}

	return hexKey, nil
}

// 生成指定长度的随机字母数字字符串
func randAlphanumeric(n int) string {
	const alphanumeric = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = alphanumeric[rand.Intn(len(alphanumeric))]
	}
	return string(b)
}

// G79HttpEncrypt G79 HTTP 数据加密（AES-CBC 无填充）
func G79HttpEncrypt(data string) (string, error) {
	dataBytes := []byte(data)

	// 步骤1：添加16位随机尾缀
	tail := randAlphanumeric(16)
	padded := append(dataBytes, []byte(tail)...)

	// 步骤2：填充到16字节倍数（补0x00）
	paddedLen := len(padded) + (16 - len(padded)%16)%16
	padded = append(padded, make([]byte, paddedLen-len(padded))...)

	// 步骤3：生成版本和随机密钥索引
	version := uint8(0x4)
	index := uint8(rand.Intn(16))

	// 步骤4：生成16位随机IV
	iv := make([]byte, 16)
	if _, err := rand.Read(iv); err != nil {
		return "", fmt.Errorf("generate iv: %w", err)
	}

	// 步骤5：获取加密密钥
	key, err := G79PickKey(index)
	if err != nil {
		return "", fmt.Errorf("pick key: %w", err)
	}

	// 步骤6：AES-CBC 加密
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create aes cipher: %w", err)
	}
	mode := cipher.NewCBCEncrypter(block, iv)
	encrypted := make([]byte, len(padded))
	mode.CryptBlocks(encrypted, padded)

	// 步骤7：拼接IV+密文+版本信息（最后一字节：index<<4 | version）
	result := make([]byte, 0, 16+len(encrypted)+1)
	result = append(result, iv...)
	result = append(result, encrypted...)
	result = append(result, index<<4|version)

	return hex.EncodeToString(result), nil
}

// G79HttpDecrypt G79 HTTP 数据解密（AES-CBC 无填充）
func G79HttpDecrypt(data string) (string, error) {
	// 步骤1：十六进制解码
	bytes, err := hex.DecodeString(data)
	if err != nil {
		return "", fmt.Errorf("decode hex data: %w", err)
	}
	if len(bytes) < 17 { // 至少需要IV(16) + 1字节信息
		return "", fmt.Errorf("data too short: len=%d", len(bytes))
	}

	// 步骤2：解析版本和密钥索引
	info := bytes[len(bytes)-1]
	version := info & 0x0F
	if version != 0x4 {
		return "", fmt.Errorf("invalid version: %d", version)
	}
	index := info >> 4

	// 步骤3：提取IV和密文
	iv := bytes[:16]
	encrypted := bytes[16 : len(bytes)-1]

	// 步骤4：获取解密密钥
	key, err := G79PickKey(index)
	if err != nil {
		return "", fmt.Errorf("pick key: %w", err)
	}

	// 步骤5：AES-CBC 解密
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create aes cipher: %w", err)
	}
	mode := cipher.NewCBCDecrypter(block, iv)
	decrypted := make([]byte, len(encrypted))
	mode.CryptBlocks(decrypted, encrypted)

	// 步骤6：去除填充（找到最后一个非0字节，往前减16位尾缀）
	dropPos := len(decrypted) - 1
	for dropPos >= 0 && decrypted[dropPos] == 0 {
		dropPos--
	}
	if dropPos < 15 { // 至少需要保留16位尾缀前的有效数据
		return "", fmt.Errorf("invalid decrypted data (no valid content)")
	}
	dropPos -= 16

	return string(decrypted[:dropPos+1]), nil
}

func PeAuthSign(v string, sp, tr int) (string, error) {
	b := []byte{
		0x62, 0x25, 0x1e, 0xf6, 0x40, 0xb3, 0x40, 0xc0, 0x51, 0x5a, 0x5e, 0x26, 0xaa, 0xc7, 0xb6, 0xe9,
		0x44, 0xea, 0xbe, 0xa4, 0xa9, 0xcf, 0xde, 0x4b, 0x60, 0x4b, 0xbb, 0xf6, 0x70, 0xbc, 0xbf, 0xbe,
		0xc3, 0x59, 0x5b, 0x65, 0x92, 0xcc, 0x0c, 0x8f, 0x7d, 0xf4, 0xef, 0xff, 0xd1, 0x5d, 0x84, 0x85,
		0xc6, 0x7e, 0x9b, 0x28, 0xfa, 0x27, 0xa1, 0xea, 0x85, 0x30, 0xef, 0xd4, 0x05, 0x1d, 0x88, 0x04,
		0xe6, 0xcd, 0xe1, 0x21, 0xd6, 0x07, 0x37, 0xc3, 0x87, 0x0d, 0xd5, 0xf4, 0xed, 0x14, 0x5a, 0x45,
	}
	qTable := []byte{
		0x01, 0x06, 0x0a, 0x0d, 0x02, 0x05, 0x09, 0x0e, 0x04, 0x07, 0x0b, 0x03, 0x03, 0x08, 0x0b, 0x05,
		0x01, 0x07, 0x0b, 0x0e,
	}

	if sp < 0 || tr <= 0 {
		return "", fmt.Errorf("invalid sp/tr parameters")
	}
	if 16*sp+16 > len(b) {
		return "", fmt.Errorf("sp=%d out of range for constant table", sp)
	}
	if 4*sp+4 > len(qTable) {
		return "", fmt.Errorf("sp=%d out of range for shift table", sp)
	}

	// pad string to 4-byte boundary
	if rem := len(v) % 4; rem != 0 {
		v += strings.Repeat("0", 4-rem)
	}

	a := []byte(v)
	p := make([]uint32, 0, len(a)/4)
	for i := 0; i < len(a); i += 4 {
		t := uint32(a[i])<<24 | uint32(a[i+1])<<16 | uint32(a[i+2])<<8 | uint32(a[i+3])
		p = append(p, t)
	}

	if rem := len(p) % 64; rem != 0 {
		for i := 0; i < 64-rem; i++ {
			p = append(p, 0xabcde987)
		}
	}

	c := make([]uint32, 4)
	for i := 0; i < 4; i++ {
		offset := 16*sp + i*4
		c[i] = binary.LittleEndian.Uint32(b[offset : offset+4])
	}
	q := make([]byte, 4)
	copy(q, qTable[4*sp:4*sp+4])

	r := uint32(0x67452301)
	u := uint32(0xefcdab89)
	x := uint32(0x98badcfe)
	z := uint32(0x10325476)

	f := func(a int64) uint32 {
		return uint32(uint64(a) & 0xffffffff)
	}

	for t := 0; t < tr; t++ {
		for j := 0; j < len(p); j += 4 {
			a1 := int64(r) + int64((u&x)|(^u&z))
			r = f(int64(u) + ((a1 + int64(p[j]) + int64(c[0])) << uint(q[0])))

			a2 := int64(r) + int64((u&z)|(^z&x))
			u = f(int64(u) + ((a2 + int64(p[j]) + int64(c[1])) << uint(q[1])))

			a3 := int64(r) + int64(u^x^z)
			x = f(int64(u) + ((a3 + int64(p[j]) + int64(c[2])) << uint(q[2])))

			term := f(int64(x) ^ int64(u|z))
			a4 := int64(r) + int64(term)
			z = f(int64(u) + ((a4 + int64(p[j]) + int64(c[3])) << uint(q[3])))
		}
	}

	out := make([]byte, 16)
	binary.LittleEndian.PutUint32(out[0:4], r)
	binary.LittleEndian.PutUint32(out[4:8], u)
	binary.LittleEndian.PutUint32(out[8:12], x)
	binary.LittleEndian.PutUint32(out[12:16], z)

	return base64.StdEncoding.EncodeToString(out), nil
}