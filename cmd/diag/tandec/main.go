// tandec 解析网易 TanLobby 明文包（帧 #1906/#1915 的加密数据实为明文）。
// 用法: tandec <hex>
package main

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

func main() {
	for _, arg := range os.Args[1:] {
		h := strings.ReplaceAll(arg, " ", "")
		b, err := hex.DecodeString(h)
		if err != nil {
			fmt.Println("hex 错误:", err)
			continue
		}
		parse(b)
		fmt.Println()
	}
}

func parse(b []byte) {
	pos := 0
	for pos < len(b) {
		if pos+2 > len(b) {
			fmt.Printf("剩余 %d 字节无法解析\n", len(b)-pos)
			break
		}
		id := binary.BigEndian.Uint16(b[pos : pos+2])
		pos += 2
		fmt.Printf("=== 包 ID=%d (%s) ===\n", id, name(id))
		switch id {
		case 0: // TanLoginRequest
			if pos+4 > len(b) {
				break
			}
			playerID := binary.BigEndian.Uint32(b[pos : pos+4])
			pos += 4
			fmt.Printf("  PlayerID=%d\n", playerID)
			if pos+32 > len(b) {
				break
			}
			fmt.Printf("  Rand=%x\n", b[pos:pos+16])
			pos += 16
			fmt.Printf("  AESRand=%x\n", b[pos:pos+16])
			pos += 16
			n := int(binary.BigEndian.Uint16(b[pos : pos+2]))
			pos += 2
			if pos+n <= len(b) {
				fmt.Printf("  PlayerName=%q\n", string(b[pos:pos+n]))
				pos += n
			}
		case 1: // TanCreateRoomRequest
			capv := b[pos]
			pos++
			privacy := b[pos]
			pos++
			fmt.Printf("  Capacity=%d Privacy=%d\n", capv, privacy)
			n := int(binary.BigEndian.Uint16(b[pos : pos+2]))
			pos += 2
			fmt.Printf("  Name=%q\n", string(b[pos:pos+n]))
			pos += n
			// Tips
			n = int(binary.BigEndian.Uint16(b[pos : pos+2]))
			pos += 2
			levelID := string(b[pos : pos+n])
			pos += n
			gt := b[pos]
			pos++
			n = int(binary.BigEndian.Uint16(b[pos : pos+2]))
			pos += 2
			cts := string(b[pos : pos+n])
			pos += n
			vioce := int16(binary.BigEndian.Uint16(b[pos : pos+2]))
			pos += 2
			pid := b[pos]
			pos++
			n = int(binary.BigEndian.Uint16(b[pos : pos+2]))
			pos += 2
			vs := string(b[pos : pos+n])
			pos += n
			fmt.Printf("  Tips{LevelID=%q GameType=%d Constant=%q Vioce=%d ProtocolID=%d Version=%q}\n", levelID, gt, cts, vioce, pid, vs)
			// ItemIDs
			n = int(binary.BigEndian.Uint16(b[pos : pos+2]))
			pos += 2
			fmt.Printf("  ItemIDs 长度=%d\n", n)
			if n > 0 {
				items := make([]uint64, 0, n/8)
				for i := 0; i < n/8; i++ {
					items = append(items, binary.BigEndian.Uint64(b[pos:pos+8]))
					pos += 8
				}
				fmt.Printf("  ItemIDs=%v\n", items)
			}
			if pos+4 > len(b) {
				break
			}
			minLevel := binary.BigEndian.Uint32(b[pos : pos+4])
			pos += 4
			fmt.Printf("  MinLevel=%d\n", minLevel)
			if pos >= len(b) {
				break
			}
			pvp := b[pos]
			pos++
			fmt.Printf("  PvP=%d\n", pvp)
			if pos+8 > len(b) {
				break
			}
			teamID := binary.BigEndian.Uint64(b[pos : pos+8])
			pos += 8
			fmt.Printf("  TeamID=%d\n", teamID)
			if pos+4 > len(b) {
				break
			}
			playerAuth := binary.BigEndian.Uint32(b[pos : pos+4])
			pos += 4
			fmt.Printf("  PlayerAuth=%d\n", playerAuth)
			n = int(binary.BigEndian.Uint16(b[pos : pos+2]))
			pos += 2
			fmt.Printf("  Password=%q\n", string(b[pos:pos+n]))
			pos += n
			n = int(binary.BigEndian.Uint16(b[pos : pos+2]))
			pos += 2
			fmt.Printf("  Slogan=%q\n", string(b[pos:pos+n]))
			pos += n
			if pos+8 > len(b) {
				break
			}
			mapID := binary.BigEndian.Uint64(b[pos : pos+8])
			pos += 8
			fmt.Printf("  MapID=%d\n", mapID)
			if pos >= len(b) {
				break
			}
			webrtc := b[pos]
			pos++
			fmt.Printf("  EnableWebRTC=%d\n", webrtc)
			if pos >= len(b) {
				break
			}
			ping := b[pos]
			pos++
			if pos >= len(b) {
				break
			}
			perf := b[pos]
			pos++
			fmt.Printf("  OwnerPing=%d PerfLv=%d\n", ping, perf)
			fmt.Printf("  剩余 %d 字节: %x\n", len(b)-pos, b[pos:])
		default:
			fmt.Printf("  未知包, 剩余 %d 字节: %x\n", len(b)-pos, b[pos:])
			return
		}
	}
}

func name(id uint16) string {
	switch id {
	case 0:
		return "TanLoginRequest"
	case 1:
		return "TanCreateRoomRequest"
	case 3:
		return "TanEnterRoomRequest"
	case 7:
		return "TanNotifyServerReady"
	}
	return "?"
}
