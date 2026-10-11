package netease

import (
	crand "crypto/rand"
	"math/big"
)

// hexCharset 与官方启动器 Random.nextString 的默认字符集一致。
const hexCharset = "0123456789abcde"

// randomString 生成 n 个字符, 字符集为 charset。
func randomString(n int, charset string) string {
	if n <= 0 || charset == "" {
		return ""
	}
	b := make([]byte, n)
	max := big.NewInt(int64(len(charset)))
	for i := range b {
		idx, err := crand.Int(crand.Reader, max)
		if err != nil {
			panic(err)
		}
		b[i] = charset[idx.Int64()]
	}
	return string(b)
}

// randomBytes 返回 n 个密码学随机字节。
func randomBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := crand.Read(b); err != nil {
		panic(err)
	}
	return b
}

// randomMAC 复刻官方启动器 RandomUtils.nextMacAddress：
// 6 个随机字节, 首字节置位本地管理地址 (mac[0] = (mac[0] & 0xFE) | 0x02), 大写冒号分隔。
func randomMAC() string {
	mac := randomBytes(6)
	mac[0] = (mac[0] & 0xFE) | 0x02
	const hexDigits = "0123456789ABCDEF"
	out := make([]byte, 0, 17)
	for i, b := range mac {
		if i > 0 {
			out = append(out, ':')
		}
		out = append(out, hexDigits[b>>4], hexDigits[b&0x0F])
	}
	return string(out)
}
