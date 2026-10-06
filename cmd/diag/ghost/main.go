// ghost 幽灵玩家：用真实客户端账号的凭据加入房主创建的房间（真实网易客户端当房主），
// 重放真实客户端捕获到的 RequestNetworkSettings + Login，抓取真实房主的响应
// （重点是 ServerToClientHandshake），与 Geyser 的握手对比。
package main

import (
	"bytes"
	"compress/flate"
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	funauth "NeteaseBedrockGateway/internal/auth"
	"NeteaseBedrockGateway/internal/room"

	"github.com/Happy2018new/nemc-tan-lobby-solver/bunker/auth"
	"github.com/Happy2018new/nemc-tan-lobby-solver/core/nethernet"
	"github.com/Happy2018new/nemc-tan-lobby-solver/protocol/login"
	g79 "github.com/Yeah114/g79client"
)

type localAccess struct {
	roomID  string
	ownerID uint32
	login   *room.CreateResult
	raknet  string
	ws      string
}

func (l *localAccess) GetRoomID() string       { return l.roomID }
func (l *localAccess) GetRoomPasscode() string { return "" }

func (l *localAccess) GetAccess() (auth.TanLobbyLoginResponse, error) {
	return auth.TanLobbyLoginResponse{
		Success:         true,
		RoomOwnerID:     l.ownerID,
		UserUniqueID:    l.login.UserUniqueID,
		UserPlayerName:  l.login.UserPlayerName,
		RaknetRand:      l.login.RaknetRand,
		RaknetAESRand:   l.login.RaknetAESRand,
		EncryptKeyBytes: l.login.EncryptKeyBytes,
		DecryptKeyBytes: l.login.DecryptKeyBytes,
		SignalingSeed:   l.login.SignalingSeed,
		SignalingTicket: l.login.SignalingTicket,
	}, nil
}

func (l *localAccess) TransferServerList() (auth.TanLobbyTransferServersResponse, error) {
	return auth.TanLobbyTransferServersResponse{
		Success:          true,
		RaknetServers:    []string{l.raknet},
		WebsocketServers: []string{l.ws},
	}, nil
}

func main() {
	var (
		username = flag.String("u", "", "4399 用户名（幽灵玩家，须与房主不同）")
		password = flag.String("p", "", "4399 密码")
		roomID   = flag.String("room-id", "", "房主房间号（真实客户端创建的）")
		loginHex = flag.String("login-hex", "", "真实客户端 Login 消息 hex 文件")
		rnsHex   = flag.String("rns-hex", "06c1010000035c", "RequestNetworkSettings hex（默认协议860）")
		timeout  = flag.Duration("timeout", 90*time.Second, "超时")
	)
	flag.Parse()
	if *username == "" || *password == "" || *roomID == "" || *loginHex == "" {
		fmt.Fprintln(os.Stderr, "用法: ghost -u 用户名 -p 密码 -room-id 房间号 -login-hex login8416.hex")
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))

	fmt.Fprintln(os.Stderr, "[1/5] 4399 登录 + x19 认证 ...")
	cli, cookie, err := funauth.Login(ctx, *username, *password, funauth.LoginOptions{})
	if err != nil {
		fatalf("认证失败: %v", err)
	}
	fmt.Fprintf(os.Stderr, "[1/5] 认证成功: uid=%s sdkuid=%s\n", cli.UserID, cookie.SDKUid)

	fmt.Fprintf(os.Stderr, "[2/5] 查询房间 %s ...\n", *roomID)
	roomInfo, err := cli.GetTransferRoomWithName(*roomID)
	if err != nil {
		fatalf("查询房间失败: %v", err)
	}
	if len(roomInfo.List) == 0 {
		fatalf("房间不存在")
	}
	target := roomInfo.List[0]
	ownerID := uint32(target.HID.Int64())
	roomTransferServerID := int(target.SRV.Int64())
	fmt.Fprintf(os.Stderr, "[2/5] 房间找到: OwnerID(HID)=%d SRV=%d\n", ownerID, roomTransferServerID)

	servers, err := g79.GetGlobalG79TransferServers()
	if err != nil {
		fatalf("获取中转列表失败: %v", err)
	}
	var raknetAddr, wsAddr string
	for _, s := range servers {
		if int(s.ID.Int64()) == roomTransferServerID {
			if len(s.Ports) > 0 {
				raknetAddr = fmt.Sprintf("%s:%d", s.IP, s.Ports[0])
			}
			if s.SignalWebPort.Int64() > 0 {
				wsAddr = fmt.Sprintf("%s:%d", s.IP, s.SignalWebPort.Int64())
			}
			break
		}
	}
	if raknetAddr == "" || wsAddr == "" {
		fatalf("未找到房间中转服务器（SRV=%d）", roomTransferServerID)
	}

	fmt.Fprintln(os.Stderr, "[3/5] 生成加入凭据 ...")
	playerRes, err := room.Create(ctx, cli)
	if err != nil {
		fatalf("生成凭据失败: %v", err)
	}

	fmt.Fprintln(os.Stderr, "[4/5] 加入房间（login.Dial → NetherNet 连房主）...")
	aw := &localAccess{roomID: *roomID, ownerID: ownerID, login: playerRes, raknet: raknetAddr, ws: wsAddr}
	netConn, err := login.Dial(aw)
	if err != nil {
		fatalf("加入失败: %v", err)
	}
	fmt.Fprintln(os.Stderr, "★★★ 已连接房主（NetherNet）★★★")
	defer netConn.Close()

	// 读取真实客户端捕获的 Login 原始字节（ReadPacket 输出，不含分段头）
	loginHexData, err := os.ReadFile(*loginHex)
	if err != nil {
		fatalf("读取 login hex 失败: %v", err)
	}
	loginBytes, err := hex.DecodeString(string(loginHexData))
	if err != nil {
		fatalf("login hex 解析失败: %v", err)
	}
	fmt.Fprintf(os.Stderr, "[5/5] Login 载荷 %d 字节\n", len(loginBytes))

	// 发送 RequestNetworkSettings（真实客户端协议 860）
	rnsBytes, _ := hex.DecodeString(*rnsHex)
	fmt.Fprintf(os.Stderr, ">>> 发送 RequestNetworkSettings(860): %x\n", rnsBytes)
	if _, err := netConn.Write(rnsBytes); err != nil {
		fatalf("写 RNS 失败: %v", err)
	}

	nc, _ := netConn.(*nethernet.Conn)
	// 读取响应 1（NetworkSettingsResponse）
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		_ = nc.SetReadDeadline(deadline)
		data, err := nc.ReadPacket()
		if err != nil {
			fatalf("读取响应1失败: %v", err)
		}
		fmt.Printf(">>> 房主响应1 (%d字节): %x\n", len(data), data)
		break
	}

	// 真实房主协商 NONE 压缩。真实客户端对 NONE 房主发送的是【无前缀 deflate】
	// （最新捕获的 Login 正是这个格式：ec 7d ...）。原样重放即可。
	loginPayload := loginBytes
	if len(loginBytes) > 0 && loginBytes[0] == 0x00 {
		// 旧捕获（[00][deflate]）→ 剥掉 00 前缀
		loginPayload = loginBytes[1:]
		fmt.Fprintf(os.Stderr, ">>> Login 剥掉 00 前缀（%d 字节）\n", len(loginPayload))
	}
	fmt.Fprintf(os.Stderr, ">>> 发送 Login (%d字节, 前16: %x)\n", len(loginPayload), loginPayload[:min(16, len(loginPayload))])
	if _, err := netConn.Write(loginPayload); err != nil {
		fatalf("写 Login 失败: %v", err)
	}

	// 持续读取房主后续响应（握手等），dump 前 8 条（若为 [0x00][deflate] 则解压显示）
	readEnd := time.Now().Add(40 * time.Second)
	for i := 0; i < 8 && time.Now().Before(readEnd); i++ {
		_ = nc.SetReadDeadline(readEnd)
		data, err := nc.ReadPacket()
		if err != nil {
			fmt.Printf(">>> 读取结束: %v\n", err)
			break
		}
		if len(data) > 0 && data[0] == 0x00 {
			zr := flate.NewReader(bytes.NewReader(data[1:]))
			frames, derr := io.ReadAll(zr)
			if derr == nil {
				fmt.Printf(">>> 房主响应%d (%d字节, 解压%d): ff%x\n", i+2, len(data), len(frames), frames)
				continue
			}
		}
		fmt.Printf(">>> 房主响应%d (%d字节): %x\n", i+2, len(data), data)
	}
	fmt.Fprintln(os.Stderr, "完成")
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "错误: "+format+"\n", args...)
	os.Exit(1)
}
