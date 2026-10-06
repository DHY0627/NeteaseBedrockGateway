package main

import (
	"encoding/hex"
	"testing"
)

// 用线上抓到的真实帧验证 bedrockPacketID。
// 这些 hex 直接取自 Orange Pi 上 03:17 那次运行的日志。
func TestBedrockPacketID(t *testing.T) {
	cases := []struct {
		name string
		hex  string
		want uint16
		ok   bool
	}{
		// 压缩开启前的 NetworkSettings：fe + [len=0c] + 8f(=143 NetworkSettings)
		{"NetworkSettings(未压缩)", "fe0c8f0100020000000000000000", 143, true},
		// ResourcePackStack：fe 00 + deflate
		{"ResourcePackStack", "fe00b367676060543132323531b6344ed1b5303136d035314d32d0b530b648d44d4a31b634b730b2483637336535d433d43366e030d43302b28c0c18c00000", 7, true},
		// 断开前最后一个包
		{"断前最后一包", "fe00e3ce67f9f9e7ffffdf8e0c950c00", 111, true},
		// 非 0xFE 开头
		{"非RakNet帧", "00ed7ddd76aa4a97", 0, false},
		// 太短
		{"太短", "fe00", 0, false},
	}
	for _, c := range cases {
		raw, err := hex.DecodeString(c.hex)
		if err != nil {
			t.Fatalf("%s: hex 解码失败: %v", c.name, err)
		}
		got, ok := bedrockPacketID(raw)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("%s: got (%d,%v), want (%d,%v) [%s]",
				c.name, got, ok, c.want, c.ok, bedrockPacketName(got))
			continue
		}
		if ok {
			t.Logf("%s -> ID=%d (%s)", c.name, got, bedrockPacketName(got))
		} else {
			t.Logf("%s -> 未识别（符合预期）", c.name)
		}
	}
}
