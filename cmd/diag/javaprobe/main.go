// javaprobe：直接对 Java 版服务器（Velocity / Paper）做状态查询或离线登录，
// 用来独立验证「Geyser → Velocity」这一跳的行为（不经过网易客户端）。
//
// 用法：
//
//	go run ./cmd/javaprobe -addr example.com:25565 -mode status
//	go run ./cmd/javaprobe -addr example.com:25565 -mode login -name 锕钼铽镧锶
package main

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"
)

func main() {
	addr := flag.String("addr", "example.com:25565", "Java 服务器地址")
	mode := flag.String("mode", "status", "status | login")
	name := flag.String("name", "TestPlayer", "登录名")
	proto := flag.Int("protocol", 0, "协议版本（0=先用状态查询自动获取）")
	uuidHex := flag.String("uuid", "", "登录用的 UUID（32位hex，不带横线；留空则用离线UUID）")
	flag.Parse()

	if *proto == 0 {
		p, info := status(*addr)
		fmt.Printf("[状态] 原始: %s\n", info)
		if p == 0 {
			fmt.Println("[状态] 无法获取协议版本")
		} else {
			fmt.Printf("[状态] 协议版本=%d\n", p)
			*proto = p
		}
	}
	if *mode == "status" {
		return
	}
	if *proto == 0 {
		fmt.Println("需要 -protocol")
		os.Exit(1)
	}

	login(*addr, *proto, *name, *uuidHex)
}

// status 做一次状态查询，返回 version.protocol。
func status(addr string) (int, string) {
	conn, err := net.DialTimeout("tcp", addr, 8*time.Second)
	if err != nil {
		fmt.Println("[状态] 连接失败:", err)
		return 0, ""
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(8 * time.Second))

	host, portStr, _ := net.SplitHostPort(addr)
	var port int
	fmt.Sscanf(portStr, "%d", &port)

	var hs bytes.Buffer
	writeVarInt(&hs, 0)
	writeVarInt(&hs, 0) // 协议版本（状态查询里无所谓）
	writeString(&hs, host)
	binary.Write(&hs, binary.BigEndian, uint16(port))
	writeVarInt(&hs, 1) // next state = status
	if err := writePacket(conn, hs.Bytes()); err != nil {
		fmt.Println("[状态] 写失败:", err)
		return 0, ""
	}
	var req bytes.Buffer
	writeVarInt(&req, 0)
	if err := writePacket(conn, req.Bytes()); err != nil {
		fmt.Println("[状态] 写失败:", err)
		return 0, ""
	}

	pkt, err := readPacket(bufio.NewReader(conn), nil)
	if err != nil {
		fmt.Println("[状态] 读失败:", err)
		return 0, ""
	}
	r := newReader(pkt)
	id, _ := readVarInt(r)
	js, _ := readString(r)
	_ = id
	var parsed struct {
		Version struct {
			Name     string `json:"name"`
			Protocol int    `json:"protocol"`
		} `json:"version"`
		Players struct {
			Online int `json:"online"`
			Max    int `json:"max"`
		} `json:"players"`
	}
	_ = json.Unmarshal([]byte(js), &parsed)
	return parsed.Version.Protocol, js
}

// login 用离线模式登录并打印服务器返回的每一个包。
func login(addr string, proto int, name, uuidHex string) {
	conn, err := net.DialTimeout("tcp", addr, 8*time.Second)
	if err != nil {
		fmt.Println("[登录] 连接失败:", err)
		return
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))

	host, portStr, _ := net.SplitHostPort(addr)
	var port int
	fmt.Sscanf(portStr, "%d", &port)

	var hs bytes.Buffer
	writeVarInt(&hs, 0)
	writeVarInt(&hs, proto)
	writeString(&hs, host)
	binary.Write(&hs, binary.BigEndian, uint16(port))
	writeVarInt(&hs, 2) // next state = login
	if err := writePacket(conn, hs.Bytes()); err != nil {
		fmt.Println("[登录] 写握手失败:", err)
		return
	}

	var ls bytes.Buffer
	writeVarInt(&ls, 0)
	writeString(&ls, name)
	u := offlineUUID(name)
	switch {
	case proto >= 764: // 1.20.2+：name + UUID
		if uuidHex != "" {
			b, _ := hex.DecodeString(strings.ReplaceAll(uuidHex, "-", ""))
			if len(b) == 16 {
				copy(u[:], b)
			}
		}
		ls.Write(u[:])
	case proto >= 761: // 1.19.3–1.20.1
		writeVarInt(&ls, 0) // has sig data = false
	default:
		writeVarInt(&ls, 0)
		writeVarInt(&ls, 0)
		writeVarInt(&ls, 0)
	}
	if err := writePacket(conn, ls.Bytes()); err != nil {
		fmt.Println("[登录] 写 LoginStart 失败:", err)
		return
	}
	fmt.Printf("[登录] 已发送 Handshake(proto=%d) + LoginStart(name=%q, UUID=%s)\n", proto, name, uuidString(u))

	threshold := -1
	for i := 0; i < 12; i++ {
		pkt, err := readPacket(bufio.NewReader(conn), &threshold)
		if err != nil {
			fmt.Printf("[登录] 连接结束: %v\n", err)
			return
		}
		r := newReader(pkt)
		id, _ := readVarInt(r)
		rest, _ := io.ReadAll(r)
		fmt.Printf("[登录] <- 包 id=0x%02x 长度=%d hex=%s\n", id, len(pkt), truncHex(pkt, 96))

		switch id {
		case 0x03: // SetCompression
			th, _ := readVarInt(newReader(rest))
			threshold = th
			fmt.Printf("          SetCompression: threshold=%d（后续包将压缩）\n", th)
		case 0x00: // 登录阶段：Disconnect
			reason, _ := readString(newReader(rest))
			fmt.Printf("          ★ Disconnect 原因: %s\n", readable(reason))
			return
		case 0x01:
			fmt.Printf("          EncryptionRequest（服务器要求正版验证）\n")
			return
		case 0x02:
			fmt.Printf("          LoginSuccess（服务器已接受登录！）\n")
			return
		}
	}
}

func readable(s string) string {
	if s == "" {
		return "(空)"
	}
	var v interface{}
	if json.Unmarshal([]byte(s), &v) == nil {
		b, _ := json.Marshal(v)
		return string(b)
	}
	return s
}

func truncHex(b []byte, n int) string {
	if len(b) > n {
		return hex.EncodeToString(b[:n]) + "..."
	}
	return hex.EncodeToString(b)
}

func uuidString(u [16]byte) string {
	return fmt.Sprintf("%x-%x-%x-%x-%x", u[0:4], u[4:6], u[6:8], u[8:10], u[10:16])
}

// offlineUUID 复刻 Java 版离线模式 UUID：nameUUIDFromBytes("OfflinePlayer:"+name)，版本3（MD5）。
func offlineUUID(name string) [16]byte {
	h := md5.Sum([]byte("OfflinePlayer:" + name))
	var u [16]byte
	copy(u[:], h[:])
	u[6] = (u[6] & 0x0f) | 0x30 // 版本 3
	u[8] = (u[8] & 0x3f) | 0x80 // IETF 变体
	return u
}

// ---- 协议读写 ----

func writeVarInt(w io.Writer, v int) {
	uv := uint32(v)
	for {
		b := byte(uv & 0x7f)
		uv >>= 7
		if uv != 0 {
			b |= 0x80
		}
		w.Write([]byte{b})
		if uv == 0 {
			return
		}
	}
}

func writeString(w io.Writer, s string) {
	writeVarInt(w, len(s))
	w.Write([]byte(s))
}

func readVarInt(r *bufio.Reader) (int, error) {
	var v uint32
	var shift uint
	for i := 0; i < 5; i++ {
		b, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		v |= uint32(b&0x7f) << shift
		if b&0x80 == 0 {
			return int(int32(v)), nil
		}
		shift += 7
	}
	return 0, fmt.Errorf("varint 过长")
}

func readString(r *bufio.Reader) (string, error) {
	n, err := readVarInt(r)
	if err != nil {
		return "", err
	}
	if n < 0 || n > 1<<21 {
		return "", fmt.Errorf("字符串长度异常: %d", n)
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(r, b); err != nil {
		return "", err
	}
	return string(b), nil
}

func newReader(b []byte) *bufio.Reader { return bufio.NewReader(bytes.NewReader(b)) }

func writePacket(w io.Writer, payload []byte) error {
	var buf bytes.Buffer
	writeVarInt(&buf, len(payload))
	buf.Write(payload)
	_, err := w.Write(buf.Bytes())
	return err
}

// readPacket 读一个包；threshold>=0 时按压缩格式解（0 表示未压缩负载）。
func readPacket(r *bufio.Reader, threshold *int) ([]byte, error) {
	length, err := readVarInt(r)
	if err != nil {
		return nil, err
	}
	if length <= 0 || length > 1<<24 {
		return nil, fmt.Errorf("包长度异常: %d", length)
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	if threshold == nil || *threshold < 0 {
		return body, nil
	}
	br2 := newReader(body)
	dataLen, err := readVarInt(br2)
	if err != nil {
		return nil, err
	}
	if dataLen == 0 {
		rest, _ := io.ReadAll(br2)
		return rest, nil
	}
	comp, _ := io.ReadAll(br2)
	zr, err := zlib.NewReader(bytes.NewReader(comp))
	if err != nil {
		return nil, fmt.Errorf("inflate: %w", err)
	}
	defer zr.Close()
	out := make([]byte, dataLen)
	if _, err := io.ReadFull(zr, out); err != nil {
		return nil, fmt.Errorf("inflate: %w", err)
	}
	return out, nil
}
