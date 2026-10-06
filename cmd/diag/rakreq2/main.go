// rakreq2 穷举 OpenConnectionRequest2 的 cookie 位置，找出 Geyser 接受的那种。
package main

import (
	"encoding/binary"
	"fmt"
	"net"
	"time"
)

var magic = []byte{0x00, 0xff, 0xff, 0x00, 0xfe, 0xfe, 0xfe, 0xfe, 0xfd, 0xfd, 0xfd, 0xfd, 0x12, 0x34, 0x56, 0x78}

func buildReq2(addr *net.UDPAddr, cookie []byte, mtu uint16, position string) []byte {
	req2 := []byte{0x07}
	req2 = append(req2, magic...)
	addrBytes := append([]byte{4}, addr.IP.To4()...)
	addrBytes = append(addrBytes, byte(addr.Port>>8), byte(addr.Port))
	mtuBytes := []byte{byte(mtu >> 8), byte(mtu)}
	guid := make([]byte, 8)
	binary.BigEndian.PutUint64(guid, 12345)

	switch position {
	case "addr_cookie_mtu":
		req2 = append(req2, addrBytes...)
		req2 = append(req2, cookie...)
		req2 = append(req2, mtuBytes...)
		req2 = append(req2, guid...)
	case "cookie_addr_mtu":
		req2 = append(req2, cookie...)
		req2 = append(req2, addrBytes...)
		req2 = append(req2, mtuBytes...)
		req2 = append(req2, guid...)
	case "addr_mtu_cookie_guid":
		req2 = append(req2, addrBytes...)
		req2 = append(req2, mtuBytes...)
		req2 = append(req2, cookie...)
		req2 = append(req2, guid...)
	}
	return req2
}

func main() {
	addr, _ := net.ResolveUDPAddr("udp", "example.com:19132")
	var cookie []byte
	var mtu uint16

	// 1. 握手拿 cookie
	conn, _ := net.DialUDP("udp", nil, addr)
	req1 := append([]byte{0x05}, magic...)
	req1 = append(req1, 11)
	req1 = append(req1, make([]byte, 1492-len(req1))...)
	conn.Write(req1)
	conn.SetDeadline(time.Now().Add(3 * time.Second))
	buf := make([]byte, 2048)
	n, err := conn.Read(buf)
	if err != nil {
		fmt.Println("Reply1 失败:", err)
		return
	}
	pos := 1 + 16 + 8 + 1
	rest := buf[pos:n]
	if len(rest) >= 6 {
		cookie = rest[0:4]
		mtu = binary.BigEndian.Uint16(rest[4:6])
		fmt.Printf("Reply1: cookie=%x mtu=%d\n", cookie, mtu)
	}
	conn.Close()

	// 2. 尝试每种 Request2
	positions := []string{"addr_cookie_mtu", "cookie_addr_mtu", "addr_mtu_cookie_guid"}
	for _, p := range positions {
		conn2, _ := net.DialUDP("udp", nil, addr)
		conn2.SetDeadline(time.Now().Add(2 * time.Second))
		req2 := buildReq2(addr, cookie, mtu, p)
		conn2.Write(req2)
		n, err := conn2.Read(buf)
		if err != nil {
			fmt.Printf("[%s] 无响应\n", p)
		} else {
			fmt.Printf("[%s] ★ 收到 Reply2 (%d字节): %x\n", p, n, buf[:n])
		}
		conn2.Close()
	}
}
