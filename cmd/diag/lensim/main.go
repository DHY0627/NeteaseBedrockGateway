// lensim 计算各种 TanLobby 包明文编码长度，用于对比真实客户端抓包帧 110（98B 明文）。
package main

import (
	"bytes"
	"fmt"

	"github.com/Happy2018new/nemc-tan-lobby-solver/protocol/encoding"
	"github.com/Happy2018new/nemc-tan-lobby-solver/protocol/packet"
)

func sizeOf(pk packet.Packet) int {
	buf := bytes.NewBuffer(nil)
	w := encoding.NewWriter(buf, 0)
	pk.Marshal(w)
	// 加 2 字节包 ID 头
	return buf.Len() + 2
}

func main() {
	// 真实房主广播内容
	fmt.Printf("TanNotifyServerReady(真实字段): %d B\n", sizeOf(&packet.TanNotifyServerReady{
		ServerAddress:         "10.0.2.15|19146",
		ServerRaknetGuid:      "",
		RTCRoomID:             "",
		NetherNetID:           "13474182814309180418",
		WebRTCCompressEnabled: true,
	}))

	// 目标: 98B 明文 (101B - 3B fee301)
	fmt.Println("目标明文长度: 98B (帧110) 或 50B (帧103: 53-3)")

	// 候选: 各种开始请求
	// 帧103 = 50B 明文 -> TanCreateRoomRequest?
	fmt.Printf("TanCreateRoomRequest(最小): %d B\n", sizeOf(&packet.TanCreateRoomRequest{
		Capacity: 8, Privacy: 0, Name: "x", Tips: encoding.RoomTips{LevelID: "", GameType: 0, ConstantTestString: "test", Vioce: 0, ProtocolID: 42, VersionString: "1.21.120.0"},
		MinLevel: 0, PlayerAuth: 0, Password: "", Slogan: "", MapID: 0, EnableWebRTC: true,
	}))

	// 帧110候选: TanNotifyServerReady + RoomID 前缀
	buf := bytes.NewBuffer(nil)
	w := encoding.NewWriter(buf, 0)
	roomID := uint32(736218)
	w.Uint32(&roomID)
	ready := &packet.TanNotifyServerReady{
		ServerAddress:         "10.0.2.15|19146",
		ServerRaknetGuid:      "",
		RTCRoomID:             "",
		NetherNetID:           "13474182814309180418",
		WebRTCCompressEnabled: true,
	}
	ready.Marshal(w)
	fmt.Printf("RoomID + TanNotifyServerReady(真实字段): %d B\n", buf.Len()+2)

	// 候选: OwnerID + RoomID + TanNotifyServerReady
	buf2 := bytes.NewBuffer(nil)
	w2 := encoding.NewWriter(buf2, 0)
	owner := uint32(3063529045)
	w2.Uint32(&owner)
	w2.Uint32(&roomID)
	ready.Marshal(w2)
	fmt.Printf("OwnerID + RoomID + TanNotifyServerReady: %d B\n", buf2.Len()+2)

	// 候选: TanCreateRoomRequest 完整（真实客户端房间名、Tip 等）
	fmt.Printf("TanCreateRoomRequest(完整): %d B\n", sizeOf(&packet.TanCreateRoomRequest{
		Capacity:     8,
		Privacy:      0,
		Name:         "bed09fdb1471的世界", // 用户在客户端创建的房间名
		Tips:         encoding.RoomTips{LevelID: "bed09fdb1471", GameType: 0, ConstantTestString: "test", Vioce: 0, ProtocolID: 42, VersionString: "1.21.120.0"},
		ItemIDs:      []uint64{4683436057584510245},
		MinLevel:     0,
		PvP:          false,
		TeamID:       0,
		PlayerAuth:   0,
		Password:     "",
		Slogan:       "来和我一起玩吧！",
		MapID:        4683436057584510245,
		EnableWebRTC: true,
		OwnerPing:    0,
		PerfLv:       0,
	}))

	// 候选: TanEnterRoomRequest（玩家进房）
	fmt.Printf("TanEnterRoomRequest(完整): %d B\n", sizeOf(&packet.TanEnterRoomRequest{
		OwnerID:               3063529045,
		RoomID:                roomID,
		EnterPassword:         "",
		EnterTeamID:           0,
		EnterToken:            0,
		FollowTeamID:          0,
		NetherNetID:           "13474182814309180418",
		SupportWebRTCCompress: true,
	}))

	// TanLoginRequest（真实字段：PlayerID + Rand16 + AESRand16 + Name）
	fmt.Printf("TanLoginRequest(实际): %d B\n", sizeOf(&packet.TanLoginRequest{
		PlayerID:   3063529045,
		Rand:       make([]byte, 16),
		AESRand:    make([]byte, 16),
		PlayerName: "testuser",
	}))

	// 目标: TanCreateRoomRequest = 104B - 3(fee301) = 101B 明文
	fmt.Println("目标: TanCreateRoomRequest 明文 101B (#412)")
	fmt.Printf("我们当前: %d B (name=debug-len slogan=24B map=0 ownerping=3 perf=3)\n", sizeOf(&packet.TanCreateRoomRequest{
		Capacity: 8, Privacy: 0, Name: "debug-len",
		Tips:     encoding.RoomTips{LevelID: "r6YhQdny6LU=", GameType: 0, ConstantTestString: "test", Vioce: 0, ProtocolID: 42, VersionString: "1.21.120.0"},
		MinLevel: 0, PvP: false, PlayerAuth: 0, Password: "", Slogan: "来和我一起玩吧！", MapID: 0, EnableWebRTC: true, OwnerPing: 3, PerfLv: 3,
	}))
	// 变体1: name=房间号(6B) slogan 保留
	fmt.Printf("变体1 (name=235847 slogan保留): %d B\n", sizeOf(&packet.TanCreateRoomRequest{
		Capacity: 10, Privacy: 0, Name: "235847",
		Tips:     encoding.RoomTips{LevelID: "rR8Qem75qD0=", GameType: 0, ConstantTestString: "test", Vioce: 0, ProtocolID: 42, VersionString: "1.21.120.0"},
		MinLevel: 0, PvP: false, PlayerAuth: 0, Password: "", Slogan: "来和我一起玩吧！", MapID: 0, EnableWebRTC: true, OwnerPing: 3, PerfLv: 3,
	}))
	// 变体2: 无 Slogan
	fmt.Printf("变体2 (name=235847 无slogan): %d B\n", sizeOf(&packet.TanCreateRoomRequest{
		Capacity: 10, Privacy: 0, Name: "235847",
		Tips:     encoding.RoomTips{LevelID: "rR8Qem75qD0=", GameType: 0, ConstantTestString: "test", Vioce: 0, ProtocolID: 42, VersionString: "1.21.120.0"},
		MinLevel: 0, PvP: false, PlayerAuth: 0, Password: "", Slogan: "", MapID: 0, EnableWebRTC: true, OwnerPing: 3, PerfLv: 3,
	}))
	// 变体3: 无 MapID 字段? 结构里 MapID 是 uint64 无法省略, 试 Name=235847 + 短 name
	fmt.Printf("变体3 (name=短 无slogan): %d B\n", sizeOf(&packet.TanCreateRoomRequest{
		Capacity: 10, Privacy: 0, Name: "x",
		Tips:     encoding.RoomTips{LevelID: "rR8Qem75qD0=", GameType: 0, ConstantTestString: "test", Vioce: 0, ProtocolID: 42, VersionString: "1.21.120.0"},
		MinLevel: 0, PvP: false, PlayerAuth: 0, Password: "", Slogan: "", MapID: 0, EnableWebRTC: true, OwnerPing: 3, PerfLv: 3,
	}))
}
