package netease

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// 官方 MpayLoginEncrypt.encryptLoginParams 的算法：
// AES(设备密钥)/ECB/PKCS7("{\"username\":..,\"password\":md5(密码),\"unique_id\":..}") → 小写 hex
func TestEncryptLoginParamsRoundTrip(t *testing.T) {
	keyHex := "00112233445566778899aabbccddeeff"
	device := &Device{UniqueID: "uni-1", ID: "dev-1", Key: keyHex, Project: "x19"}

	got, err := encryptLoginParams(device, "someone@example.com", "p@ssw0rd")
	if err != nil {
		t.Fatalf("encryptLoginParams: %v", err)
	}
	if len(got)%32 != 0 {
		t.Fatalf("密文 hex 长度应为 32 的倍数，实际 %d", len(got))
	}
	if strings.ToLower(got) != got {
		t.Fatalf("hex 应为小写: %s", got)
	}

	key, err := device.aesKey()
	if err != nil {
		t.Fatalf("aesKey: %v", err)
	}
	plain, err := aesECBPKCS7Decrypt(got, key)
	if err != nil {
		t.Fatalf("解密失败: %v", err)
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
		UniqueID string `json:"unique_id"`
	}
	if err := json.Unmarshal(plain, &body); err != nil {
		t.Fatalf("解密结果不是 JSON: %v (%s)", err, plain)
	}
	if body.Username != "someone@example.com" {
		t.Errorf("username = %q", body.Username)
	}
	if body.Password != md5hex("p@ssw0rd") {
		t.Errorf("password 应为 md5，得到 %q", body.Password)
	}
	if body.UniqueID != "uni-1" {
		t.Errorf("unique_id = %q", body.UniqueID)
	}
}

// 同一个明文用同一个密钥加密两次结果必须一致（ECB、无随机 IV）。
func TestAESECBPKCS7HexDeterministic(t *testing.T) {
	key := []byte("0123456789abcdef")
	a, err := aesECBPKCS7Hex([]byte("hello world"), key)
	if err != nil {
		t.Fatalf("aesECBPKCS7Hex: %v", err)
	}
	b, _ := aesECBPKCS7Hex([]byte("hello world"), key)
	if a != b {
		t.Fatalf("ECB 加密结果不稳定: %s vs %s", a, b)
	}
	// "hello world" 占 11 字节，PKCS7 补满一个 16 字节分组 → hex 32 字符
	if len(a) != 32 {
		t.Fatalf("密文长度应为 32 个 hex 字符，实际 %d", len(a))
	}
	plain, err := aesECBPKCS7Decrypt(a, key)
	if err != nil || string(plain) != "hello world" {
		t.Fatalf("回环失败: %v %q", err, plain)
	}
}

func TestAESECBPKCS7RejectsBadCipher(t *testing.T) {
	key := []byte("0123456789abcdef")
	if _, err := aesECBPKCS7Decrypt("xyz", key); err == nil {
		t.Fatal("非 hex 密文应当报错")
	}
	if _, err := aesECBPKCS7Decrypt("0011", key); err == nil {
		t.Fatal("长度不足一个分组的密文应当报错")
	}
}

var macPattern = regexp.MustCompile(`^(?:[0-9A-F]{2}:){5}[0-9A-F]{2}$`)

func TestRandomMACFormat(t *testing.T) {
	for i := 0; i < 50; i++ {
		mac := randomMAC()
		if !macPattern.MatchString(mac) {
			t.Fatalf("MAC 格式不对: %q", mac)
		}
		// 首字节的本地管理位必须置 1（组播位必须为 0）
		first := mac[0:2]
		v, err := strconv.ParseUint(first, 16, 8)
		if err != nil {
			t.Fatalf("解析首字节失败: %v", err)
		}
		if v&0x02 == 0 {
			t.Fatalf("首字节应带本地管理位: %s", mac)
		}
		if v&0x01 != 0 {
			t.Fatalf("首字节不应是组播地址: %s", mac)
		}
	}
}

func TestRandomStringCharset(t *testing.T) {
	for i := 0; i < 20; i++ {
		s := randomString(16, hexCharset)
		if len(s) != 16 {
			t.Fatalf("长度应为 16: %q", s)
		}
		for _, r := range s {
			if !strings.ContainsRune(hexCharset, r) {
				t.Fatalf("出现非法字符 %q in %q", r, s)
			}
		}
	}
	if len(randomBytes(8)) != 8 {
		t.Fatal("randomBytes 长度不对")
	}
}

func TestDeviceAesKeyAcceptsPlainText(t *testing.T) {
	d := &Device{Key: "0123456789abcdef"}
	if _, err := d.aesKey(); err != nil {
		t.Fatalf("16 字符明文密钥应当可用: %v", err)
	}
	d = &Device{Key: "00112233445566778899aabbccddeeff"}
	if _, err := d.aesKey(); err != nil {
		t.Fatalf("32 位 hex 密钥应当可用: %v", err)
	}
	d = &Device{Key: "short"}
	if _, err := d.aesKey(); err == nil {
		t.Fatal("非法密钥应当报错")
	}
}
