// 加入测试工具：4399 登录 → 查询房间 → 通过网易中转加入房间 → 连接房主（NetherNet）。
//
// 用途：验证"玩家通过房间号加入"链路。用与房主不同的 4399 账号运行，
// 输入房主创建的 RoomID，观察能否成功加入（收到 TanNotifyServerReady 并建立连接）。
//
// 注意：网易可能拒绝"同账号加入自己创建的房间"，因此测试时必须使用
// 与房主不同的账号。
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	funauth "NeteaseBedrockGateway/internal/auth"
	"NeteaseBedrockGateway/internal/room"

	"github.com/Happy2018new/nemc-tan-lobby-solver/bunker/auth"
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
		username = flag.String("u", "", "4399 用户名（加入者，须与房主不同）")
		password = flag.String("p", "", "4399 密码")
		roomID   = flag.String("room-id", "", "要加入的房间号（房主创建返回的 RoomID）")
		timeout  = flag.Duration("timeout", 120*time.Second, "超时")
	)
	flag.Parse()
	if *username == "" || *password == "" || *roomID == "" {
		fmt.Fprintln(os.Stderr, "用法: join -u 用户名 -p 密码 -room-id 房间号")
		flag.Usage()
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	// 1. 4399 登录 + x19 认证
	fmt.Fprintln(os.Stderr, "[1/4] 4399 登录 + x19 认证 ...")
	cli, cookie, err := funauth.Login(ctx, *username, *password, funauth.LoginOptions{})
	if err != nil {
		fatalf("认证失败: %v", err)
	}
	fmt.Fprintf(os.Stderr, "[1/4] 认证成功: uid=%s sdkuid=%s\n", cli.UserID, cookie.SDKUid)

	// 2. 查询房间，拿到房主ID（HID）和房间所在中转（SRV）
	fmt.Fprintf(os.Stderr, "[2/4] 查询房间 %s ...\n", *roomID)
	roomInfo, err := cli.GetTransferRoomWithName(*roomID)
	if err != nil {
		fatalf("查询房间失败: %v", err)
	}
	if len(roomInfo.List) == 0 {
		fatalf("房间不存在（请确认房主在线且房间号正确）")
	}
	target := roomInfo.List[0]
	ownerID := uint32(target.HID.Int64())
	roomTransferServerID := int(target.SRV.Int64())
	fmt.Fprintf(os.Stderr, "[2/4] 房间找到: OwnerID(HID)=%d SRV=%d RoomUniqueID=%s\n",
		ownerID, roomTransferServerID, target.RoomUniqueID)

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
	fmt.Fprintf(os.Stderr, "[2/4] 房间中转=%s 房间信令=%s\n", raknetAddr, wsAddr)

	// 3. 生成加入凭据
	fmt.Fprintln(os.Stderr, "[3/4] 生成加入凭据 ...")
	playerRes, err := room.Create(ctx, cli)
	if err != nil {
		fatalf("生成凭据失败: %v", err)
	}

	// 4. 完整加入流程（login.Dial：进房 → 信令 → NetherNet 连房主）
	fmt.Fprintln(os.Stderr, "[4/4] 加入房间 ...")
	aw := &localAccess{
		roomID:  *roomID,
		ownerID: ownerID,
		login:   playerRes,
		raknet:  raknetAddr,
		ws:      wsAddr,
	}
	netConn, err := login.Dial(aw)
	if err != nil {
		fatalf("加入失败: %v", err)
	}
	fmt.Fprintln(os.Stderr, "★★★ 加入成功！已连接房主（NetherNet）★★★")
	defer netConn.Close()

	// 读取数据（预期是 Minecraft 协议数据）
	buf := make([]byte, 8192)
	for i := 0; i < 10; i++ {
		n, err := netConn.Read(buf)
		if err != nil {
			fmt.Fprintf(os.Stderr, "读取结束: %v\n", err)
			break
		}
		fmt.Fprintf(os.Stderr, "收到数据 %d 字节\n", n)
	}
}

var _ = bytes.NewBuffer
var _ = fmt.Sprintf

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "错误: "+format+"\n", args...)
	os.Exit(1)
}
