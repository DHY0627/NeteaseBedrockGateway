package wplauncher

import (
	crand "crypto/rand"
	"math/big"
)

// hexCharset 与 Kotlin 端 Random.nextString 的默认字符集一致
const hexCharset = "0123456789abcde"

// diskCharset 与 Kotlin 端 Environment.disk 使用的字符集一致 (注意没有 D)
const diskCharset = "0123456789ABCEF"

// randomString 生成 n 个字符, 字符集为 charset
func randomString(n int, charset string) string {
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

func randomBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := crand.Read(b); err != nil {
		panic(err)
	}
	return b
}

// xorWow 复刻 kotlin.random.XorWowRandom (Random(seed) 在 JVM 上的实现)
type xorWow struct {
	x, y, z, w uint32
}

func newXorWow(seed int64) *xorWow {
	return &xorWow{
		x: uint32(seed) | 1,
		y: uint32(seed>>32) | 1,
		z: 1,
		w: 0,
	}
}

func (r *xorWow) nextBits(bitCount int) uint32 {
	x, y, z, w := r.x, r.y, r.z, r.w
	t := x ^ (x << 11)
	r.x, r.y, r.z, r.w = y, z, w, (w^(w>>19))^(t^(t>>8))
	return r.w >> (32 - bitCount)
}

// nextInt 复刻 kotlin Random.nextInt(until), until=16 时无拒绝采样
func (r *xorWow) nextInt(until int) int {
	return int(r.nextBits(31) % uint32(until))
}

// diskFromAid 复刻 Kotlin: Random(aid.toLong()).nextString(6, "0123456789ABCEF")
func diskFromAid(aid uint64) string {
	r := newXorWow(int64(aid))
	b := make([]byte, 6)
	for i := range b {
		b[i] = diskCharset[r.nextInt(len(diskCharset))]
	}
	return string(b)
}
