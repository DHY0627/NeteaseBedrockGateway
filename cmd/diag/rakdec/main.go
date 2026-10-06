// rakdec 简易 RakNet 帧解析器：从 pcap 提取 UDP 载荷，解析 datagram + encapsulation，
// 输出每个 encapsulation 的 content（hex），并尝试识别网易批量头 fee301。
// 用法: rakdec <file.pcap> [port]
package main

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
)

func readUint24(b []byte) uint32 {
	return uint32(b[0])<<16 | uint32(b[1])<<8 | uint32(b[2])
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "用法: rakdec <file.pcap> [port]")
		os.Exit(2)
	}
	f, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()

	targetPort := ""
	if len(os.Args) >= 3 {
		targetPort = os.Args[2]
	}

	hdr := make([]byte, 24)
	if _, err := f.Read(hdr); err != nil {
		fmt.Fprintln(os.Stderr, "读全局头失败:", err)
		os.Exit(1)
	}
	magic := binary.LittleEndian.Uint32(hdr[0:4])
	// 用文件字节序读 magic：文件字节 d4 c3 b2 a1 (LE 写入) 时 magic=0xa1b2c3d4 -> 小端文件
	// 文件字节 a1 b2 c3 d4 (BE 写入) 时 magic=0xd4c3b2a1 -> 大端文件
	bigEndian := magic == 0xd4c3b2a1
	if magic != 0xa1b2c3d4 && magic != 0xd4c3b2a1 {
		fmt.Fprintf(os.Stderr, "未知 pcap magic 0x%08x\n", magic)
		os.Exit(1)
	}
	if bigEndian {
		fmt.Println("pcap: big-endian")
	} else {
		fmt.Println("pcap: little-endian")
	}

	body := make([]byte, 65536)
	var pktSeq int
	for {
		ph := make([]byte, 16)
		if _, err := f.Read(ph); err != nil {
			break
		}
		if pktSeq <= 1 {
			fmt.Printf("[dbg] pcap-record-header %s\n", hex.EncodeToString(ph))
		}
		var tsSec, tsUsec, inclLen, origLen uint32
		if bigEndian {
			tsSec = binary.BigEndian.Uint32(ph[0:4])
			tsUsec = binary.BigEndian.Uint32(ph[4:8])
			inclLen = binary.BigEndian.Uint32(ph[8:12])
			origLen = binary.BigEndian.Uint32(ph[12:16])
		} else {
			tsSec = binary.LittleEndian.Uint32(ph[0:4])
			tsUsec = binary.LittleEndian.Uint32(ph[4:8])
			inclLen = binary.LittleEndian.Uint32(ph[8:12])
			origLen = binary.LittleEndian.Uint32(ph[12:16])
		}
		if inclLen > uint32(len(body)) {
			inclLen = uint32(len(body))
		}
		if _, err := f.Read(body[:inclLen]); err != nil {
			break
		}
		pktSeq++
		_ = tsSec
		_ = tsUsec
		_ = origLen

		if pktSeq <= 4 || pktSeq == 93 || pktSeq == 101 {
			fmt.Printf("[dbg] #%d inclLen=%d first16=%s\n", pktSeq, inclLen, hex.EncodeToString(body[:min(16, int(inclLen))]))
		}

		if inclLen < 20+20+8 {
			continue
		}
		// Linux cooked-mode capture v2 (SLL2): 20 字节头
		// 协议类型在偏移 0-1 (BE)，0x0800 = IPv4, 0x86dd = IPv6
		etherType := binary.BigEndian.Uint16(body[0:2])
		var ipStart, udpStart int
		var srcIP, dstIP string
		if etherType == 0x0800 {
			ipStart = 20
			if body[ipStart+9] != 17 {
				continue
			}
			srcIP = fmt.Sprintf("%d.%d.%d.%d", body[ipStart+12], body[ipStart+13], body[ipStart+14], body[ipStart+15])
			dstIP = fmt.Sprintf("%d.%d.%d.%d", body[ipStart+16], body[ipStart+17], body[ipStart+18], body[ipStart+19])
			udpStart = ipStart + 20
		} else if etherType == 0x86dd {
			ipStart = 20
			if body[ipStart+6] != 17 {
				continue
			}
			srcIP = "v6:" + hex.EncodeToString(body[ipStart+8:ipStart+24])
			dstIP = "v6:" + hex.EncodeToString(body[ipStart+24:ipStart+40])
			udpStart = ipStart + 40
		} else {
			continue
		}
		if udpStart+8 > int(inclLen) {
			continue
		}
		sp := binary.BigEndian.Uint16(body[udpStart : udpStart+2])
		dp := binary.BigEndian.Uint16(body[udpStart+2 : udpStart+4])
		if targetPort != "" && fmt.Sprint(sp) != targetPort && fmt.Sprint(dp) != targetPort {
			continue
		}
		payload := body[udpStart+8 : inclLen]
		if len(payload) < 4 {
			continue
		}
		if pktSeq == 3 || pktSeq == 93 || pktSeq == 101 || pktSeq == 103 || pktSeq == 110 {
			fmt.Printf("[dbg] #%d payload[0:16]=%s len=%d\n", pktSeq, hex.EncodeToString(payload[:min(16, len(payload))]), len(payload))
		}
		_ = payload

		// RakNet datagram: 首字节 flags, 3 字节 sequence (LE)
		flags := payload[0]
		if flags&0x80 == 0 && flags&0x40 == 0 && flags&0x20 == 0 {
			// 不是 datagram/ACK/NACK（可能是握手包）
			if flags == 0x05 || flags == 0x06 || flags == 0x07 || flags == 0x08 {
				fmt.Printf("#%d %s:%d -> %s:%d [握手 %02x] %s\n", pktSeq, srcIP, sp, dstIP, dp, flags, hex.EncodeToString(payload[:min(len(payload), 24)]))
			}
			continue
		}
		if flags&0x40 != 0 {
			// ACK
			fmt.Printf("#%d %s:%d -> %s:%d [ACK]\n", pktSeq, srcIP, sp, dstIP, dp)
			continue
		}
		if flags&0x20 != 0 {
			fmt.Printf("#%d %s:%d -> %s:%d [NACK]\n", pktSeq, srcIP, sp, dstIP, dp)
			continue
		}
		seq := readUint24(payload[1:4])
		rest := payload[4:]
		// 解析 encapsulation
		pos := 0
		for pos < len(rest) {
			encapHeader := rest[pos]
			reliability := (encapHeader & 224) >> 5
			split := (encapHeader & 0x10) != 0
			pos++
			if pos+2 > len(rest) {
				break
			}
			bitLen := binary.BigEndian.Uint16(rest[pos : pos+2])
			contentLen := int(bitLen >> 3)
			pos += 2
			if reliability == 2 || reliability == 3 || reliability == 4 {
				if pos+3 > len(rest) {
					break
				}
				pos += 3 // message index
			}
			if reliability == 1 || reliability == 3 || reliability == 4 {
				if pos+3 > len(rest) {
					break
				}
				pos += 3 // sequence/order index
				pos++    // order channel
			}
			if split {
				if pos+10 > len(rest) {
					break
				}
				pos += 10 // split count/id/index
			}
			if pos+contentLen > len(rest) {
				contentLen = len(rest) - pos
			}
			content := rest[pos : pos+contentLen]
			pos += contentLen

			// 识别网易批量头 fee301
			desc := "content"
			bodyHex := hex.EncodeToString(content)
			if len(content) >= 3 && content[0] == 0xfe && content[1] == 0xe3 && content[2] == 0x01 {
				desc = "NeteaseBatch(fee301)"
			}
			fmt.Printf("#%d %s:%d -> %s:%d [datagram seq=%d rel=%d split=%v] %s len=%d: %s\n",
				pktSeq, srcIP, sp, dstIP, dp, seq, reliability, split, desc, contentLen, bodyHex)
		}
	}
}
