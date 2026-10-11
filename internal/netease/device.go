// Package netease 实现网易账号（手机号 / 邮箱 / SAuth）登录《我的世界》中国版。
//
// 本包是 DHY0627/NeteaseLogin（neteasex19）在 NeteaseBedrockGateway 内的移植版：
// 不引入额外 module 依赖，直接产出 internal/wplauncher.Sauth，
// 从而复用网关既有的 x19 认证链（internal/auth），登录成功后照常创建房间。
//
// 与 4399 渠道的区别只在「先拿到 sauth 登录态」这一步：
//
//	4399 渠道 : 4399 OAuth（表单 + 验证码）→ Cookie4399
//	网易渠道 : mpay 账号中台（设备注册 + 邮箱/手机号登录）→ SauthCookie
//
// 之后的 uni_sauth → login-otp → authentication-otp 完全一致。
//
// 支持四种登录方式：
//
//	LoginByEmail         网易邮箱 + 密码
//	LoginByPhonePassword 手机号 + 密码
//	LoginByPhoneSms      手机号 + 短信验证码（含风控要求的上行短信）
//	LoginBySauth         直接使用一段已有的 sauth_json
package netease

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Device 是 mpay 侧的设备指纹。
//
// 网易对同一 unique_id 重复注册会触发风控, 因此注册结果应当缓存复用:
// 拿 DeviceCache 存下来, 下次登录直接 Load 出来即可。
type Device struct {
	// UniqueID 客户端生成的设备唯一 id (32 位 hex)
	UniqueID string `json:"unique_id"`
	// ID 服务端返回的设备 id, 形如 "amawvsyac6x5dsbv-d"
	ID string `json:"id"`
	// Key 服务端返回的设备密钥 (hex 字符串, 解码后为 16 字节 AES-128 密钥)
	Key string `json:"key"`
	// Project 注册时使用的 mpay project
	Project string `json:"project,omitempty"`
	// GameVersion 注册时使用的游戏版本
	GameVersion string `json:"game_version,omitempty"`
}

// aesKey 返回设备密钥的字节形式。
// 服务端下发的是 16 字节密钥的 hex 编码; 少数情况下也可能是 16 字符明文, 这里做兼容。
func (d *Device) aesKey() ([]byte, error) {
	if d.Key == "" {
		return nil, fmt.Errorf("设备 %s 缺少密钥", d.ID)
	}
	if raw, err := hex.DecodeString(d.Key); err == nil && len(raw) == 16 {
		return raw, nil
	}
	if len(d.Key) == 16 {
		return []byte(d.Key), nil
	}
	return nil, fmt.Errorf("设备密钥长度非法: %q", d.Key)
}

// usable 判断缓存设备能否用于指定 project。
func (d *Device) usable(project string) bool {
	return d != nil && d.ID != "" && d.UniqueID != "" && d.Key != "" &&
		(d.Project == "" || project == "" || d.Project == project)
}

// DeviceCache 设备缓存, 用于复用已注册设备、避免重复注册触发风控。
type DeviceCache interface {
	// Load 读取缓存的设备, 未缓存时返回 (nil, nil)
	Load() (*Device, error)
	// Save 写入设备
	Save(device *Device) error
}

// MemoryDeviceCache 内存缓存 (进程内复用)。
type MemoryDeviceCache struct {
	mu     sync.Mutex
	device *Device
}

// NewMemoryDeviceCache 创建内存设备缓存。
func NewMemoryDeviceCache() *MemoryDeviceCache { return &MemoryDeviceCache{} }

// Load 实现 DeviceCache。
func (c *MemoryDeviceCache) Load() (*Device, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.device == nil {
		return nil, nil
	}
	clone := *c.device
	return &clone, nil
}

// Save 实现 DeviceCache。
func (c *MemoryDeviceCache) Save(device *Device) error {
	if device == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	clone := *device
	c.device = &clone
	return nil
}

// FileDeviceCache 文件缓存, 默认用于跨进程复用设备指纹。
type FileDeviceCache struct {
	// Path JSON 文件路径
	Path string

	mu sync.Mutex
}

// NewFileDeviceCache 创建文件设备缓存。
func NewFileDeviceCache(path string) *FileDeviceCache {
	return &FileDeviceCache{Path: path}
}

// DefaultDeviceCacheDir 返回默认的设备缓存目录: <用户目录>/.neteasebedrockgateway。
func DefaultDeviceCacheDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "."
	}
	return filepath.Join(home, ".neteasebedrockgateway")
}

// DefaultDeviceCachePath 返回默认缓存路径: <用户目录>/.neteasebedrockgateway/device.json。
func DefaultDeviceCachePath() string {
	return filepath.Join(DefaultDeviceCacheDir(), "device.json")
}

// Load 实现 DeviceCache。
func (c *FileDeviceCache) Load() (*Device, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	b, err := os.ReadFile(c.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var d Device
	if err := json.Unmarshal(b, &d); err != nil {
		// 缓存损坏时视作未缓存, 由调用方重新注册
		return nil, nil
	}
	return &d, nil
}

// Save 实现 DeviceCache。
func (c *FileDeviceCache) Save(device *Device) error {
	if device == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if dir := filepath.Dir(c.Path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	b, err := json.MarshalIndent(device, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.Path, b, 0o600)
}
