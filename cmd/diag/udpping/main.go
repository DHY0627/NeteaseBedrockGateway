// udpping 发送 RakNet Unconnected Ping 到服务器，验证 UDP 可达性。
package main

import (
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "用法: udpping <host:port>")
		os.Exit(2)
	}
	addr, err := net.ResolveUDPAddr("udp", os.Args[1])
	if err != nil {
		fmt.Println("解析失败:", err)
		return
	}
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		fmt.Println("连接失败:", err)
		return
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	// RakNet Unconnected Ping: 0x01 + time(8) + magic(16) + clientGUID(8)
	ping := []byte{0x01}
	now := time.Now().UnixMilli()
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(now))
	ping = append(ping, b...)
	magic := []byte{0x00, 0xff, 0xff, 0x00, 0xfe, 0xfe, 0xfe, 0xfe, 0xfd, 0xfd, 0xfd, 0xfd, 0x12, 0x34, 0x56, 0x78}
	ping = append(ping, magic...)
	ping = append(ping, 0, 0, 0, 0, 0, 0, 0, 0)

	n, err := conn.Write(ping)
	if err != nil {
		fmt.Println("发送失败:", err)
		return
	}
	fmt.Printf("已发送 %d 字节到 %s\n", n, os.Args[1])

	buf := make([]byte, 2048)
	n, err = conn.Read(buf)
	if err != nil {
		fmt.Println("无响应:", err)
		return
	}
	fmt.Printf("收到 %d 字节响应: %x\n", n, buf[:min(n, 64)])
}
