// crctest 验证 SCTP CRC32C checksum（RFC 4960 非反射算法）。
// 测试向量: 16 字节全零 -> 0x328AA689（RFC 3309 附录 A.5）。
package main

import (
	"fmt"
	"hash/crc32"
)

// 非反射 CRC32C（RFC 4960）：多项式 0x1EDC6F41，MSB-first，无反射。
func crc32cNonReflected(data []byte) uint32 {
	var crc uint32 = 0xFFFFFFFF
	for _, b := range data {
		crc ^= uint32(b) << 24
		for i := 0; i < 8; i++ {
			if crc&0x80000000 != 0 {
				crc = (crc << 1) ^ 0x1EDC6F41
			} else {
				crc <<= 1
			}
		}
	}
	return crc ^ 0xFFFFFFFF
}

func sctpChecksum(raw []byte) uint32 {
	// checksum 字段（offset 8-12）置零后计算
	tmp := make([]byte, len(raw))
	copy(tmp, raw)
	tmp[8], tmp[9], tmp[10], tmp[11] = 0, 0, 0, 0
	return crc32cNonReflected(tmp)
}

func main() {
	raw := make([]byte, 16)
	sum := sctpChecksum(raw)
	fmt.Printf("非反射 CRC32C 全零16字节 = 0x%08X (RFC期望 0x328AA689)\n", sum)
	fmt.Printf("大端字节: %02X %02X %02X %02X\n", byte(sum>>24), byte(sum>>16), byte(sum>>8), byte(sum))

	// 对比 Go 反射 Castagnoli (直接 Checksum)
	cast := crc32.Checksum(raw, crc32.MakeTable(crc32.Castagnoli))
	fmt.Printf("Go反射 Castagnoli Checksum(全16字节) = 0x%08X\n", cast)

	// 手动: checksum 字段置零后 16 字节
	zero := make([]byte, 16)
	cast2 := crc32.Checksum(zero, crc32.MakeTable(crc32.Castagnoli))
	fmt.Printf("Go Castagnoli 全零16 = 0x%08X\n", cast2)

	// SCTP: 用 IEEE 表 (原实现) 计算 16 字节全零
	ieee := crc32.Checksum(zero, crc32.MakeTable(crc32.IEEE))
	fmt.Printf("Go IEEE 全零16 = 0x%08X\n", ieee)
}
