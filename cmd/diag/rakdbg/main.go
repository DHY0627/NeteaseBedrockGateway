// rakdbg 调试 raknet 握手到 Geyser（example.com:19132），用 raw 字节。
package main

import (
	"encoding/binary"
	"fmt"
	"net"
	"time"
)

var magic = []byte{0x00, 0xff, 0xff, 0x00, 0xfe, 0xfe, 0xfe, 0xfe, 0xfd, 0xfd, 0xfd, 0xfd, 0x12, 0x34, 0x56, 0x78}

func main() {
	addr, _ := net.ResolveUDPAddr("udp", "example.com:19132")
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		fmt.Println("dial:", err)
		return
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))

	// 1. OpenConnectionRequest1: 0x05 + magic + 协议(1) + 填充
	req1 := append([]byte{0x05}, magic...)
	req1 = append(req1, 11) // protocol 11
	pad := make([]byte, 1492-len(req1))
	req1 = append(req1, pad...)
	conn.Write(req1)
	fmt.Println("发送 Request1")

	buf := make([]byte, 2048)
	n, err := conn.Read(buf)
	if err != nil {
		fmt.Println("读 Reply1 失败:", err)
		return
	}
	fmt.Printf("收到 Reply1 (%d字节): %x\n", n, buf[:n])
	if buf[0] != 0x06 {
		fmt.Println("不是 Reply1, id=", buf[0])
		return
	}
	pos := 1  // 跳过 id
	pos += 16 // magic
	guid := binary.BigEndian.Uint64(buf[pos:])
	pos += 8
	secure := buf[pos]
	pos++
	fmt.Printf("Reply1: ServerGUID=%d Secure=%d\n", guid, secure)
	rest := buf[pos:n]
	fmt.Printf("Reply1 剩余 %d 字节: %x\n", len(rest), rest)
	// 尝试: [cookie 4][mtu 2] 或 [mtu 2]
	var cookie []byte
	var mtu uint16
	if len(rest) >= 6 {
		cookie = rest[0:4]
		mtu = binary.BigEndian.Uint16(rest[4:6])
		fmt.Printf("  若 [cookie4][mtu2]: cookie=%x mtu=%d\n", cookie, mtu)
	}
	if len(rest) >= 2 {
		mtu2 := binary.BigEndian.Uint16(rest[0:2])
		fmt.Printf("  若 [mtu2]: mtu=%d\n", mtu2)
		if mtu2 == 1400 {
			mtu = mtu2
		}
	}

	// 2. OpenConnectionRequest2: 0x07 + magic + addr + [cookie] + mtu + guid
	req2 := []byte{0x07}
	req2 = append(req2, magic...)
	// server address: 版本4 + ip(4) + port(2)
	req2 = append(req2, 4)
	req2 = append(req2, addr.IP.To4()...)
	req2 = append(req2, byte(addr.Port>>8), byte(addr.Port))
	// cookie (4字节) — 在 mtu 之前
	if len(cookie) == 4 {
		req2 = append(req2, cookie...)
		fmt.Printf("Request2 带 cookie: %x\n", cookie)
	}
	req2 = append(req2, byte(mtu>>8), byte(mtu))
	guidBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(guidBytes, 12345)
	req2 = append(req2, guidBytes...)
	conn.Write(req2)
	fmt.Println("发送 Request2 (带 cookie)")

	conn.SetDeadline(time.Now().Add(3 * time.Second))
	n, err = conn.Read(buf)
	if err != nil {
		fmt.Println("读 Reply2 失败（带 cookie）:", err)
	} else {
		fmt.Printf("收到 Reply2 (%d字节): %x\n", n, buf[:n])
	}
}
