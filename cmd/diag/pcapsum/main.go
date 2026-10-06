// pcapsum 简易 pcap 分析工具：列出 UDP 流概要（源/目的端口、包数、字节数、首包时间戳）
// 用于分析网易客户端开房抓包。用法: pcapsum <file.pcap>
package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"sort"
)

type flowKey struct {
	srcIP string
	srcP  uint16
	dstIP string
	dstP  uint16
}

type flowStat struct {
	pkts int
	byts int
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "用法: pcapsum <file.pcap>")
		os.Exit(2)
	}
	f, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()

	// 全局头 24 字节
	hdr := make([]byte, 24)
	if _, err := f.Read(hdr); err != nil {
		fmt.Fprintln(os.Stderr, "读全局头失败:", err)
		os.Exit(1)
	}
	// magic: d4 c3 b2 a1 (LE) 或 a1 b2 c3 d4 (BE)
	magic := binary.LittleEndian.Uint32(hdr[0:4])
	bigEndian := magic == 0xa1b2c3d4
	fmt.Printf("pcap magic=0x%08x bigEndian=%v\n", magic, bigEndian)

	flows := map[flowKey]*flowStat{}
	var order []flowKey
	body := make([]byte, 65536)
	for {
		ph := make([]byte, 16)
		if _, err := f.Read(ph); err != nil {
			break
		}
		var tsSec, tsUsec uint32
		var inclLen, origLen uint32
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
		_ = tsSec
		_ = tsUsec
		_ = origLen

		// 解析以太网(14) + IP(20) + UDP(8)
		if inclLen < 14+20+8 {
			continue
		}
		etherType := binary.BigEndian.Uint16(body[12:14])
		var ipStart int
		if etherType == 0x0800 {
			ipStart = 14
		} else if etherType == 0x86dd {
			// IPv6: 40 字节头, next header 在 body[14+6]
			ipStart = 14
			next := body[ipStart+6]
			if next != 17 { // UDP
				continue
			}
			// 跳过 40 字节 IPv6 头
			ipStart += 40
			if inclLen < uint32(ipStart+8) {
				continue
			}
			srcIP := fmt.Sprintf("%x:%x:%x:%x:%x:%x:%x:%x",
				binary.BigEndian.Uint16(body[14+8:14+10]),
				binary.BigEndian.Uint16(body[14+10:14+12]),
				binary.BigEndian.Uint16(body[14+12:14+14]),
				binary.BigEndian.Uint16(body[14+14:14+16]),
				binary.BigEndian.Uint16(body[14+24:14+26]),
				binary.BigEndian.Uint16(body[14+26:14+28]),
				binary.BigEndian.Uint16(body[14+28:14+30]),
				binary.BigEndian.Uint16(body[14+30:14+32]),
			)
			dstIP := fmt.Sprintf("%x:%x:%x:%x:%x:%x:%x:%x",
				binary.BigEndian.Uint16(body[14+16:14+18]),
				binary.BigEndian.Uint16(body[14+18:14+20]),
				binary.BigEndian.Uint16(body[14+20:14+22]),
				binary.BigEndian.Uint16(body[14+22:14+24]),
				binary.BigEndian.Uint16(body[14+32:14+34]),
				binary.BigEndian.Uint16(body[14+34:14+36]),
				binary.BigEndian.Uint16(body[14+36:14+38]),
				binary.BigEndian.Uint16(body[14+38:14+40]),
			)
			sp := binary.BigEndian.Uint16(body[ipStart : ipStart+2])
			dp := binary.BigEndian.Uint16(body[ipStart+2 : ipStart+4])
			key := flowKey{srcIP: srcIP, srcP: sp, dstIP: dstIP, dstP: dp}
			if _, ok := flows[key]; !ok {
				flows[key] = &flowStat{}
				order = append(order, key)
			}
			flows[key].pkts++
			flows[key].byts += int(inclLen)
			continue
		} else {
			continue
		}
		if inclLen < uint32(ipStart+20+8) {
			continue
		}
		proto := body[ipStart+9]
		if proto != 17 { // UDP
			continue
		}
		srcIP := fmt.Sprintf("%d.%d.%d.%d", body[ipStart+12], body[ipStart+13], body[ipStart+14], body[ipStart+15])
		dstIP := fmt.Sprintf("%d.%d.%d.%d", body[ipStart+16], body[ipStart+17], body[ipStart+18], body[ipStart+19])
		udpStart := ipStart + 20
		sp := binary.BigEndian.Uint16(body[udpStart : udpStart+2])
		dp := binary.BigEndian.Uint16(body[udpStart+2 : udpStart+4])
		key := flowKey{srcIP: srcIP, srcP: sp, dstIP: dstIP, dstP: dp}
		if _, ok := flows[key]; !ok {
			flows[key] = &flowStat{}
			order = append(order, key)
		}
		flows[key].pkts++
		flows[key].byts += int(inclLen)
	}

	sort.Slice(order, func(i, j int) bool {
		return flows[order[i]].pkts > flows[order[j]].pkts
	})
	for _, k := range order {
		fmt.Printf("%s:%-5d -> %s:%-5d  pkts=%d bytes=%d\n",
			k.srcIP, k.srcP, k.dstIP, k.dstP, flows[k].pkts, flows[k].byts)
	}
}
