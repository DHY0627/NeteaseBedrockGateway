// 房主网关：4399 登录 → x19 认证 → 创建网易本地联机房间（Tan Lobby）
// → 连接信令服务器 → NetherNet 监听玩家 → 转发玩家流量到目标服务器（Geyser/BDS）。
//
// 已实测验证（2026-10-06，真实网易客户端 + Geyser/Velocity 目标服）：
//   - 4399 登录 + x19 认证 ✅
//   - 生成开房凭据 / 创建房间（TanCreateRoomRequest → RoomID）✅
//   - 查询房间（GetTransferRoomWithName → HID/SRV/中转）✅
//   - 连接信令服务器 + NetherNet Listener ✅
//   - 玩家 TanEnterRoom / 进房 / TanNotifyServerReady / NetherNet+SCTP ✅
//   - 玩家 Bedrock 数据双向透传到目标服，网易客户端成功进入 Java 服 ✅
//
// 待改进项（见 gateway.go）：
//   - 房间生命周期：中转断开或房间被回收时自动重建（已实现）
//   - 房间信息落盘 room.json / room.txt，便于外部读取当前房间号（已实现）
//
// 文件分工：
//   - main.go    ：协议细节（TanCreateRoom、connectTan、hostReadLoop、玩家转发、日志）
//   - gateway.go ：生命周期（登录/凭据刷新/建房间/保活/重建/持久化）
package main

import (
	"bufio"
	"bytes"
	"compress/flate"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"NeteaseBedrockGateway/internal/room"

	"github.com/Happy2018new/nemc-tan-lobby-solver/core/nethernet"
	"github.com/Happy2018new/nemc-tan-lobby-solver/core/raknet"
	"github.com/Happy2018new/nemc-tan-lobby-solver/protocol/encoding"
	"github.com/Happy2018new/nemc-tan-lobby-solver/protocol/packet"
	"github.com/pion/logging"
	gorsk "github.com/sandertv/go-raknet"
)

// netherMsgLimit 是转发给玩家的单条 NetherNet 消息上限（字节，0=不限制）。
// 由 -nether-msg-limit 设置，供 handlePlayer 使用（它不直接拿得到 flag）。
var netherMsgLimit = 0

// debugLog 由 -d / --debug 开启：输出逐帧 hex、pion(ICE/DTLS/SCTP) 与 NetherNet 信令细节。
// 关闭时只保留生命周期与关键事件日志（登录/建房间/玩家进出/保活）。
var debugLog = false

type session struct {
	raknetConn *raknet.Conn
	enc        *packet.Encoder
	dec        *packet.Decoder
}

// defaultLevelID 是房间的「游戏版本标识」，base64 编码的 8 字节值
// （解码为 AF A6 21 41 D9 F2 E8 B5），对应游戏版本 1.21.120.0。
//
// 该值与 VersionString（"1.21.120.0"）一起放在 RoomTips 里，网易客户端加入房间时
// 用它校验版本一致性。**留空会导致玩家能进房间、但无法「开始游戏」**：
// 客户端拿不到可识别的版本标识，就不会去建立 NetherNet 连接，
// 网关侧表现为只收到 TanNewGuestResponse、之后完全没有玩家拨入
// （没有任何 handleOffer / ICE 日志）。
//
// 取值来自逆向真实客户端创建房间时的报文（见 cmd/diag/lensim）。
// 若玩家使用的游戏版本不同，这个值可能需要一并更换。
const defaultLevelID = "r6YhQdny6LU="

func main() {
	var (
		username     = flag.String("u", "", "4399 用户名")
		password     = flag.String("pass", "", "4399 密码（原 -p；-p 现在表示 Web 控制台端口）")
		roomName     = flag.String("room-name", "NeteaseBedrockGateway Host Room", "房间名称")
		roomCapacity = flag.Uint("capacity", 8, "房间容量")
		roomPassword = flag.String("room-password", "", "房间密码（可留空）")
		target       = flag.String("target", "", "转发目标：玩家流量最终连到的服务器（Geyser/BDS 的 RakNet 端口，必填，例如 服务器IP/域名:49780）")
		serverAddr   = flag.String("server-address", "", "上报给网易的房主地址（玩家据此连房主）。留空则与 -target 相同；二者不同时才需要单独指定")
		mapID        = flag.Uint64("map-id", 0, "房间 MapID（游戏版本标识）")
		protocolID   = flag.Uint("protocol-id", 42, "房间 ProtocolID（默认 42 匹配真实房间）")
		levelID      = flag.String("level-id", defaultLevelID, "房间 LevelID：游戏版本标识（base64 的 8 字节）。默认值对应 1.21.120.0，留空会导致玩家进了房间却无法开始游戏")
		gameType     = flag.Uint("game-type", 0, "房间 GameType")
		versionStr   = flag.String("version-string", "1.21.120.0", "房间游戏版本字符串（玩家校验用）")
		roomFile     = flag.String("room-file", "room.json", "房间信息落盘文件（同时写同名 .txt 只存房间号；空字符串=不落盘）")
		keepalive    = flag.Duration("keepalive", 25*time.Second, "房间存活检查间隔（0=关闭；连续 3 次查不到即自动重建房间）")
		// 网易客户端在 SDP 里声明 a=max-message-size:262144（256KB）。
		// 超过它的帧一律不转发：实测 Geyser 会发来 30 万字节级的块数据帧，
		// 直接转发会让客户端在若干秒后闪退。设为 0 可关闭该保护。
		netherMsgLimitFlag = flag.Int("nether-msg-limit", 0, "诊断开关：转发给玩家的单条 NetherNet 消息上限（字节，0=不限制）。设成 262144 会把超过客户端 max-message-size 的帧【丢弃】（会丢区块，仅用于排查）")
		debugShort         = flag.Bool("d", false, "输出详细日志：逐帧 hex、pion(ICE/DTLS/SCTP)、NetherNet 信令细节")
		debugLong          = flag.Bool("debug", false, "同 -d（写成 --debug 亦可）")

		// Web 控制台（不带参数启动即进入控制台模式）
		webPort    = flag.Int("p", 8765, "Web 控制台端口（1-65535，默认 8765）")
		installSvc = flag.Bool("install", false, "注册为 systemd 服务（仅 Linux，需 root）")
		runConfig  = flag.String("run", "", "内部模式：按 JSON 配置运行单个开房实例（由 Web 控制台拉起）")
		webRoot    = flag.String("web-root", "", "Web 静态文件目录（默认自动查找 exe 旁的 web/ 或 ./web）")
	)
	flag.Parse()
	netherMsgLimit = *netherMsgLimitFlag
	debugLog = *debugShort || *debugLong

	// ---------------- 模式分派 ----------------
	// 1) -run <file>：由 Web 控制台拉起的单实例模式。参数从 JSON 读，4399 密码不出现在命令行里。
	if *runConfig != "" {
		rc, err := loadRunConfig(*runConfig)
		if err != nil {
			fatalf("读取实例配置失败: %v", err)
		}
		*username, *password = rc.Username, rc.Password
		*roomName, *roomPassword = rc.RoomName, rc.RoomPassword
		if rc.Capacity > 0 {
			*roomCapacity = rc.Capacity
		}
		*target, *serverAddr = rc.Target, rc.ServerAddress
		if rc.MapID != 0 {
			*mapID = rc.MapID
		}
		if rc.ProtocolID != 0 {
			*protocolID = rc.ProtocolID
		}
		if rc.LevelID != "" {
			*levelID = rc.LevelID
		}
		if rc.GameType != 0 {
			*gameType = rc.GameType
		}
		if rc.VersionString != "" {
			*versionStr = rc.VersionString
		}
		if rc.RoomFile != "" {
			*roomFile = rc.RoomFile
		}
		if rc.KeepaliveSeconds > 0 {
			*keepalive = time.Duration(rc.KeepaliveSeconds) * time.Second
		}
		debugLog = rc.Debug
	}
	// 2) -install：注册 systemd 服务
	if *installSvc {
		installSystemd(*webPort)
		return
	}
	// 3) 没给 -target（也没走 -run）→ 进入 Web 控制台模式
	if *runConfig == "" && *target == "" {
		serveConsole(*webPort, findWebRoot(*webRoot))
		return
	}

	if *username == "" || *password == "" || *target == "" {
		fmt.Fprintln(os.Stderr, "用法1（Web 控制台）: NeteaseBedrockGateway [-p 端口] [-install] [-web-root 目录]")
		fmt.Fprintln(os.Stderr, "用法2（单房间 CLI）: NeteaseBedrockGateway -u 用户名 -pass 密码 -target 服务器IP/域名:端口 [-server-address 房主地址] [-room-name 名称] [-capacity 容量] [-room-password 密码] [-map-id ID] [-protocol-id ID] [-level-id 版本] [-game-type 类型] [-version-string 版本字符串] [-room-file 落盘文件] [-keepalive 间隔] [-d|--debug]")
		if *username != "" && *password != "" && *target == "" {
			fmt.Fprintln(os.Stderr, "错误: -target 必填（玩家流量转发目标，例如 -target 服务器IP/域名:49780）")
		}
		flag.Usage()
		os.Exit(2)
	}
	// -server-address 是「玩家去哪找房主」，-target 是「玩家流量转发到哪」。
	// 二者绝大多数情况下相同（同一台机器、同一个 RakNet 端口），留空即继承 -target。
	if *serverAddr == "" {
		*serverAddr = *target
	}
	if !strings.Contains(*serverAddr, ":") {
		fmt.Fprintf(os.Stderr, "错误: -server-address 必须带端口，例如 %s:49780\n", *serverAddr)
		os.Exit(2)
	}
	if !strings.Contains(*target, ":") {
		fmt.Fprintf(os.Stderr, "错误: -target 必须带端口，例如 %s:49780\n", *target)
		os.Exit(2)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("收到退出信号，正在关闭房主网关 ...")
		cancel()
	}()

	// pion 各子系统（ICE/DTLS/SCTP）的 Debug 日志，便于排查连接建立过程
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))

	// 生命周期交给 gateway：登录 → 开房 → 监听服务；房间失效（中转断开/被回收）自动重建。
	log.Printf("[房主] 转发目标: %s，上报房主地址: %s，房间信息落盘: %s，存活检查间隔: %s，详细日志: %v", *target, *serverAddr, *roomFile, *keepalive, debugLog)
	g := &gateway{cfg: gatewayConfig{
		username:      *username,
		password:      *password,
		roomName:      *roomName,
		roomCapacity:  *roomCapacity,
		roomPassword:  *roomPassword,
		target:        *target,
		serverAddr:    *serverAddr,
		mapID:         *mapID,
		protocolID:    uint8(*protocolID),
		levelID:       *levelID,
		gameType:      uint8(*gameType),
		versionString: *versionStr,
		roomFile:      *roomFile,
		keepalive:     *keepalive,
	}}
	if err := g.run(ctx); err != nil {
		fatalf("%v", err)
	}
}

func createRoom(ctx context.Context, s *session, name string, capacity uint, password string, mapID uint64, protocolID uint8, levelID string, gameType uint8, versionString string, hostNetherID uint64, serverAddr string) (uint32, error) {
	// 自定义编码：标准 TanCreateRoomRequest 字段 + 尾部追加 NetherNetID + ServerAddress
	// （真实客户端在创建房间时上报房主 NetherNetID 与 ServerAddress，服务器据此向玩家广播
	// TanNotifyServerReady。solver 结构缺少这两个字段，故手动追加。）
	buf := bytes.NewBuffer(nil)
	writer := encoding.NewWriter(buf, 0)
	req := &packet.TanCreateRoomRequest{
		Capacity:     uint8(capacity),
		Privacy:      0,
		Name:         name,
		Tips:         encoding.RoomTips{LevelID: levelID, GameType: gameType, ConstantTestString: "test", Vioce: 0, ProtocolID: protocolID, VersionString: versionString},
		MinLevel:     0,
		PvP:          false,
		PlayerAuth:   0,
		Password:     password,
		Slogan:       "来和我一起玩吧！",
		MapID:        mapID,
		EnableWebRTC: true,
		OwnerPing:    3,
		PerfLv:       3,
	}
	req.Marshal(writer)
	// 追加 NetherNetID (StringUTF) 和 ServerAddress (StringUTF)
	netherIDStr := fmt.Sprintf("%d", hostNetherID)
	writer.StringUTF(&netherIDStr)
	writer.StringUTF(&serverAddr)
	log.Printf("[createRoom] 追加字段 NetherNetID=%s ServerAddress=%s 明文总长=%d\n", netherIDStr, serverAddr, buf.Len()+2)

	header := packet.Header{PacketID: packet.IDTanCreateRoomRequest}
	headerBuf := bytes.NewBuffer(nil)
	if err := header.Write(headerBuf); err != nil {
		return 0, err
	}
	if err := s.enc.Encode(append(headerBuf.Bytes(), buf.Bytes()...)); err != nil {
		return 0, fmt.Errorf("TanCreateRoomRequest: %w", err)
	}
	pk, err := readPacket(s.dec)
	if err != nil {
		return 0, fmt.Errorf("读取创建响应: %w", err)
	}
	createResp, ok := pk.(*packet.TanCreateRoomResponse)
	if !ok {
		return 0, fmt.Errorf("意外响应类型: %T", pk)
	}
	if createResp.ErrorCode != packet.TanCreateRoomSuccess {
		return 0, fmt.Errorf("创建房间被拒绝: error_code=%d", createResp.ErrorCode)
	}
	return createResp.RoomID, nil
}

// hostReadLoop 保持房主与中转的连接，并打印中转发来的任何包（调试）。
func hostReadLoop(ctx context.Context, host *session, hostNetherID uint64, serverAddr string, g *gateway) {
	for {
		pk, err := readPacket(host.dec)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			// 未知包 ID / 解析失败不应导致断开：记录并继续。
			// （部分包的线格式尚未用抓包确认，解析失败时把原始字节打出来。）
			if strings.Contains(err.Error(), "未知包 ID") || strings.Contains(err.Error(), "解析包失败") {
				log.Printf("[房主] %v（继续监听）", err)
				continue
			}
			log.Printf("[房主] 中转连接关闭: %v", err)
			return
		}
		switch p := pk.(type) {
		case *packet.TanNewGuestResponse:
			log.Printf("[房主] ★ 新玩家加入房间（ErrorCode=%d 玩家数=%d）", p.ErrorCode, len(p.PlayerIDList))
			if g != nil {
				g.onRoomPlayers(len(p.PlayerIDList))
			}
			// 玩家进房后，房主向服务器上报自己的 NetherNetID + ServerAddress，
			// 服务器据此向玩家广播 TanNotifyServerReady，玩家才能开始连接。
			if err := writePacket(host.enc, &packet.TanNotifyServerReady{
				ServerAddress:         serverAddr,
				ServerRaknetGuid:      "",
				RTCRoomID:             "",
				NetherNetID:           fmt.Sprintf("%d", hostNetherID),
				WebRTCCompressEnabled: true,
			}); err != nil {
				log.Printf("[房主] 发送 TanNotifyServerReady 失败: %v", err)
			} else {
				log.Printf("[房主] 已向玩家上报 NetherNetID=%d ServerAddress=%s", hostNetherID, serverAddr)
			}
		default:
			log.Printf("[房主] 中转收到: %T %+v", pk, pk)
		}
	}
}

// acceptPlayers 接受玩家连接，并把每个玩家转发到目标服务器。
// 玩家通过网易中转进入房间后，会通过信令/NetherNet 连接到本程序。
// 本程序作为"代理"：把玩家的 Minecraft 数据双向转发到目标服务器。
func acceptPlayers(ctx context.Context, listener *nethernet.Listener, target string, g *gateway) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			// 信令连接断开后 listener 会关闭，此时应优雅退出而非继续循环
			log.Printf("accept 错误: %v", err)
			return
		}
		if conn == nil {
			log.Printf("accept 返回 nil 连接（信令已断开），停止监听")
			return
		}
		log.Printf("★ 收到玩家连接: %s", conn.RemoteAddr())
		if nc, ok := conn.(*nethernet.Conn); ok {
			if g != nil {
				g.onPlayerServed()
			}
			go handlePlayer(ctx, nc, target, g)
		} else {
			log.Printf("连接类型意外: %T", conn)
			_ = conn.Close()
		}
	}
}

// handlePlayer 把单个玩家的 Minecraft 数据双向转发到目标服务器。
//
// 架构：
//
//	网易玩家（nethernet.Conn，承载 Minecraft 协议原始字节）
//	    ↕  字节级双向透传
//	目标服务器（example.com:19132，RakNet + Minecraft）
//
// 玩家侧 nethernet.Conn 是可靠的 Minecraft 数据通道（与 nemc-tan-lobby-solver
// 的 login.Dial 返回的连接同构），目标侧用 sandertv/go-raknet 建立 RakNet
// 连接（兼容 BDS/Geyser/cloudburst，含 Secure cookie 握手），两者之间做
// 原始字节透传，不做 packet 解析（避免协议版本/加密差异）。
func handlePlayer(ctx context.Context, playerConn *nethernet.Conn, target string, g *gateway) {
	defer playerConn.Close()
	if g != nil {
		defer g.onPlayerLeft()
	}
	log.Printf("[转发] 玩家已连接，开始转发到 %s ...", target)

	// 立即开始读取玩家数据包到缓冲（避免 dial 期间阻塞 NetherNet/SCTP 读循环）
	pending := make(chan []byte, 64)
	go func() {
		for {
			data, err := playerConn.ReadPacket()
			if err != nil {
				// 这一行是判断「哪一侧先断」的关键：以前这里是静默 return，
				// 导致 玩家连接结束 时完全看不出是谁断的、为什么断。
				log.Printf("[转发] 玩家侧数据通道结束: %v", err)
				close(pending)
				return
			}
			pending <- data
		}
	}()

	// 记录不可靠数据通道的流量（诊断用；Bedrock 用户数据理论只在可靠通道）
	unreliableDone := make(chan struct{})
	go func() {
		defer close(unreliableDone)
		for {
			data, err := playerConn.ReadUnreliablePacket()
			if err != nil {
				log.Printf("[转发] 玩家侧不可靠通道结束: %v", err)
				return
			}
			relayLogLine("[玩家→(不可靠)]", data)
		}
	}()

	// 连接目标服务器（go-raknet 兼容 Geyser/BDS 的 Secure cookie 握手）。
	// 带超时 + 重试：go-raknet 在无 deadline 的 context 下会无限重试。
	//
	// 注意这里用 *gorsk.Conn 而不是 net.Conn：下面必须用 ReadPacket() 读取，
	// 不能用 Read(buf)。详见读取循环处的说明。
	var netConn *gorsk.Conn
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		netConn, lastErr = raknetDial(ctx, target)
		if lastErr == nil {
			break
		}
		log.Printf("[转发] 连接目标服务器第 %d 次失败: %v（2 秒后重试）", attempt, lastErr)
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
	if netConn == nil {
		log.Printf("[转发] 连接目标服务器失败（3 次）: %v", lastErr)
		return
	}
	defer netConn.Close()
	log.Printf("[转发] 已连接目标服务器: %s", target)

	// 双向透传（不做 Minecraft 握手——玩家已握手的协议流原样转发）。
	//
	// 帧头说明（2026-08-31 实测确认）：
	//   - 网易客户端的 NetherNet 数据通道消息 = [分段数][Minecraft 批次]，不含 0xFE；
	//   - Geyser/BDS 的 RakNet 用户消息 = [0xFE][Minecraft 批次]，FrameIdCodec 校验首字节 0xFE；
	//   - 因此 玩家→服务器 需要补 0xFE，服务器→玩家 需要剥掉 0xFE。
	//
	// 压缩转译（2026-08-31 关键修复）：
	//   真实网易房主的 NetworkSettingsResponse = 阈值1 + NONE 压缩（`01 00 02 00...`），
	//   真实网易联机全程不压缩；而 Geyser 协商 ZLIB。网易客户端在 ZLIB 下能压缩发送，
	//   但解压服务器响应会卡死（首次解压即握手包 → 超时）。
	//   因此网关把 Geyser 的压缩响应解压后以 [0xFF][帧]（NONE 格式）转给客户端，
	//   客户端发来的未压缩 [0xFF][帧] 重新以 [0x00][raw-deflate] 压缩给 Geyser。
	var rstate relayState

	done := make(chan string, 2)

	// 玩家 → 目标服务器
	go func() {
		for data := range pending {
			if debugLog {
				log.Printf("[转发] 玩家→服务器 %d 字节: %x", len(data), data[:min(len(data), 64)])
			}
			relayLogLine("[玩家→服务器]", data)
			out := playerToServer(&rstate, data)
			if _, err := netConn.Write(out); err != nil {
				done <- fmt.Sprintf("写入目标服务器失败: %v", err)
				return
			}
		}
		done <- "玩家侧不再有数据（玩家通道已关闭）"
	}()

	// 目标服务器 → 玩家
	//
	// 必须用 ReadPacket() 而不是 Read(buf)：
	// go-raknet 的 Conn.Read 会在「传入的切片小于整个 RakNet 包」时直接返回
	// ErrBufferTooSmall（"a message sent was larger than the buffer used to
	// receive the message into"）并结束读取。块数据（LevelChunk/SubChunk）
	// 经分片重组后很容易超过 64KB，用固定缓冲区必然踩到，
	// 表现为世界生成到一半突然断线。ReadPacket 返回完整包，没有大小限制。
	go func() {
		var oversize, oversizeBytes, oversizeMax int
		for {
			pk, err := netConn.ReadPacket()
			if err != nil {
				if oversize > 0 {
					log.Printf("[转发] 本次连接共丢弃 %d 个超过 %d 字节的帧（最大 %d 字节，合计 %d 字节）",
						oversize, netherMsgLimit, oversizeMax, oversizeBytes)
				}
				done <- fmt.Sprintf("目标服务器连接结束: %v", err)
				return
			}
			n := len(pk)
			// 去掉 0xFE 帧头后才是真正写到 NetherNet 的消息长度
			msgLen := n - 1
			if netherMsgLimit > 0 && msgLen > netherMsgLimit {
				// NetherNet 单条消息不能超过对端在 SDP 里声明的 max-message-size
				// （网易客户端声明 262144）。实测 Geyser 会发来 30 万字节级的
				// 块数据帧，超过上限时客户端会在若干秒后闪退 —— 疑似其接收缓冲区
				// 按声明的上限分配，越界写坏了内存。
				oversize++
				oversizeBytes += msgLen
				if msgLen > oversizeMax {
					oversizeMax = msgLen
				}
				log.Printf("[转发] 服务器→玩家 %d 字节 超过 max-message-size(%d)，丢弃该帧（第 %d 个）",
					msgLen, netherMsgLimit, oversize)
				if id, ok := bedrockPacketID(pk); ok {
					log.Printf("[转发]   └ 被丢弃的包 ID=%d (%s)", id, bedrockPacketName(id))
				}
				continue
			}
			if debugLog {
				log.Printf("[转发] 服务器→玩家 %d 字节: %x", n, pk[:min(n, 64)])
				if id, ok := bedrockPacketID(pk); ok {
					log.Printf("[转发]   └ 包 ID=%d (%s)", id, bedrockPacketName(id))
				}
			}
			relayLogLine("[服务器→玩家]", pk)
			out, err := serverToPlayer(&rstate, pk)
			if err != nil {
				done <- fmt.Sprintf("服务器→玩家转译失败: %v", err)
				return
			}
			if _, err := playerConn.Write(out); err != nil {
				done <- fmt.Sprintf("写回玩家失败: %v", err)
				return
			}
		}
	}()

	log.Printf("[转发] 玩家连接结束（%s）", <-done)
}

// relayPhase 表示中继所处的登录阶段。
type relayPhase int

const (
	phaseBootstrap relayPhase = iota // 尚未收到 NetworkSettingsResponse（不压缩）
	phaseTranslate                   // 已协商 NONE 压缩，登录阶段（Login/握手）
	phaseEncrypted                   // 握手之后（加密，原样透传）
)

// relayState 保存压缩转译的共享状态。
type relayState struct {
	mu    sync.Mutex
	phase relayPhase
}

func (r *relayState) get() relayPhase {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.phase
}

func (r *relayState) set(p relayPhase) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.phase = p
}

// playerToServer：客户端 → Geyser。补上 RakNet 帧头 0xFE。
//
// 说明：不修改 NetworkSettingsResponse（保持 Geyser 的 ZLIB 协商），
// 这样客户端与 Geyser 双方都使用标准 [0x00][raw-deflate] 批次格式，
// 中继只需做帧头转换。加密握手由扩展侧跳过（网易局域网流程不做 Bedrock 层加密），
// 因此会话始终是明文批次。
func playerToServer(r *relayState, data []byte) []byte {
	out := make([]byte, 0, len(data)+1)
	out = append(out, 0xFE)
	out = append(out, data...)
	return out
}

// serverToPlayer：Geyser → 客户端。剥掉 RakNet 帧头 0xFE。
func serverToPlayer(r *relayState, raw []byte) ([]byte, error) {
	if len(raw) > 0 && raw[0] == 0xFE {
		return raw[1:], nil
	}
	return raw, nil
}

// readVarint 读取无符号 varint（保留给诊断工具使用）。
func readVarint(b []byte) (uint32, int) {
	var v uint32
	var shift uint
	for i := 0; i < len(b) && i < 5; i++ {
		c := b[i]
		v |= uint32(c&0x7f) << shift
		if c&0x80 == 0 {
			return v, i + 1
		}
		shift += 7
	}
	return 0, 0
}

// bedrockPacketID 尝试从一帧「服务器→玩家」数据里取出 Bedrock 包 ID，用于日志。
//
// 帧格式（已用 relay.log 实测确认）：
//
//	压缩开启前（如 NetworkSettingsResponse）：[0xFE][varint 长度][包 ID][载荷]
//	压缩开启后：                              [0xFE][0x00 = raw-deflate][deflate 数据]
//	                                         解压后为 [varint 长度][包 ID][载荷]
//
// 对压缩帧只解压出头部几十字节（flate 是流式的，读 32 字节就够，哪怕是 50KB 的块包
// 也几乎不花代价），因此不限制帧大小 —— 查明「崩溃前最后一个包是什么」正需要看大包。
//
// 失败时返回 (0,false)，绝不 panic —— 这纯粹是诊断信息，不能影响转发。
func bedrockPacketID(raw []byte) (uint16, bool) {
	if len(raw) < 4 {
		return 0, false
	}
	if raw[0] != 0xFE {
		return 0, false
	}
	var body []byte
	if raw[1] == 0x00 {
		zr := flate.NewReader(bytes.NewReader(raw[2:]))
		defer zr.Close()
		out := make([]byte, 32)
		n, _ := io.ReadFull(zr, out)
		if n < 2 {
			return 0, false
		}
		body = out[:n]
	} else {
		body = raw[1:]
	}
	// [varint 长度][包 ID]
	_, m := readVarint(body)
	if m == 0 || m >= len(body) {
		return 0, false
	}
	return uint16(body[m]), true
}

// bedrockPacketName 给常见的 Bedrock 包 ID 一个可读名字（仅用于日志）。
// ID 取自 solver 的 minecraft/protocol/packet/id.go。
func bedrockPacketName(id uint16) string {
	if n, ok := bedrockPacketNames[id]; ok {
		return n
	}
	return "未知"
}

var bedrockPacketNames = map[uint16]string{
	1:   "Login",
	2:   "PlayStatus",
	3:   "ServerToClientHandshake",
	4:   "ClientToServerHandshake",
	5:   "Disconnect",
	6:   "ResourcePacksInfo",
	7:   "ResourcePackStack",
	8:   "ResourcePackClientResponse",
	9:   "Text",
	10:  "SetTime",
	11:  "StartGame",
	12:  "AddPlayer",
	13:  "AddActor",
	14:  "RemoveActor",
	15:  "AddItemActor",
	17:  "TakeItemActor",
	18:  "MoveActorAbsolute",
	19:  "MovePlayer",
	20:  "PassengerJump",
	21:  "UpdateBlock",
	22:  "AddPainting",
	23:  "TickSync",
	25:  "LevelEvent",
	26:  "BlockEvent",
	27:  "ActorEvent",
	28:  "MobEffect",
	29:  "UpdateAttributes",
	30:  "InventoryTransaction",
	31:  "MobEquipment",
	32:  "MobArmourEquipment",
	33:  "Interact",
	34:  "BlockPickRequest",
	35:  "ActorPickRequest",
	36:  "PlayerAction",
	38:  "HurtArmour",
	39:  "SetActorData",
	40:  "SetActorMotion",
	41:  "SetActorLink",
	42:  "SetHealth",
	43:  "SetSpawnPosition",
	44:  "Animate",
	45:  "Respawn",
	46:  "ContainerOpen",
	47:  "ContainerClose",
	48:  "PlayerHotBar",
	49:  "InventoryContent",
	50:  "InventorySlot",
	51:  "ContainerSetData",
	52:  "CraftingData",
	54:  "GUIDataPickItem",
	55:  "AdventureSettings",
	56:  "BlockActorData",
	57:  "PlayerInput",
	58:  "LevelChunk",
	59:  "SetCommandsEnabled",
	60:  "SetDifficulty",
	61:  "ChangeDimension",
	62:  "SetPlayerGameType",
	63:  "PlayerList",
	64:  "SimpleEvent",
	65:  "Event",
	66:  "SpawnExperienceOrb",
	67:  "ClientBoundMapItemData",
	68:  "MapInfoRequest",
	69:  "RequestChunkRadius",
	70:  "ChunkRadiusUpdated",
	72:  "GameRulesChanged",
	73:  "Camera",
	74:  "BossEvent",
	75:  "ShowCredits",
	76:  "AvailableCommands",
	77:  "CommandRequest",
	78:  "CommandBlockUpdate",
	79:  "CommandOutput",
	80:  "UpdateTrade",
	81:  "UpdateEquip",
	82:  "ResourcePackDataInfo",
	83:  "ResourcePackChunkData",
	84:  "ResourcePackChunkRequest",
	85:  "Transfer",
	86:  "PlaySound",
	87:  "StopSound",
	88:  "SetTitle",
	89:  "AddBehaviourTree",
	90:  "StructureBlockUpdate",
	91:  "ShowStoreOffer",
	92:  "PurchaseReceipt",
	93:  "PlayerSkin",
	94:  "SubClientLogin",
	95:  "AutomationClientConnect",
	96:  "SetLastHurtBy",
	97:  "BookEdit",
	98:  "NPCRequest",
	99:  "PhotoTransfer",
	100: "ModalFormRequest",
	101: "ModalFormResponse",
	102: "ServerSettingsRequest",
	103: "ServerSettingsResponse",
	104: "ShowProfile",
	105: "SetDefaultGameType",
	106: "RemoveObjective",
	107: "SetDisplayObjective",
	108: "SetScore",
	109: "LabTable",
	110: "UpdateBlockSynced",
	111: "MoveActorDelta",
	112: "SetScoreboardIdentity",
	113: "SetLocalPlayerAsInitialised",
	114: "UpdateSoftEnum",
	115: "NetworkStackLatency",
	118: "SpawnParticleEffect",
	119: "AvailableActorIdentifiers",
	121: "NetworkChunkPublisherUpdate",
	122: "BiomeDefinitionList",
	123: "LevelSoundEvent",
	124: "LevelEventGeneric",
	125: "LecternUpdate",
	129: "ClientCacheStatus",
	130: "OnScreenTextureAnimation",
	131: "MapCreateLockedCopy",
	132: "StructureTemplateDataRequest",
	133: "StructureTemplateDataResponse",
	135: "ClientCacheBlobStatus",
	136: "ClientCacheMissResponse",
	137: "EducationSettings",
	138: "Emote",
	139: "MultiPlayerSettings",
	140: "SettingsCommand",
	141: "AnvilDamage",
	142: "CompletedUsingItem",
	143: "NetworkSettings",
	144: "PlayerAuthInput",
	145: "CreativeContent",
	146: "PlayerEnchantOptions",
	147: "ItemStackRequest",
	148: "ItemStackResponse",
	149: "PlayerArmourDamage",
	150: "CodeBuilder",
	151: "UpdatePlayerGameType",
	152: "EmoteList",
	153: "PositionTrackingDBServerBroadcast",
	154: "PositionTrackingDBClientRequest",
	155: "DebugInfo",
	156: "PacketViolationWarning",
	157: "MotionPredictionHints",
	158: "AnimateEntity",
	159: "CameraShake",
	160: "PlayerFog",
	161: "CorrectPlayerMovePrediction",
	162: "ItemComponent",
	163: "FilterText",
	164: "ClientBoundDebugRenderer",
	165: "SyncActorProperty",
	166: "AddVolumeEntity",
	167: "RemoveVolumeEntity",
	168: "SimulationType",
	169: "NPCDialogue",
	170: "EducationResourceURI",
	171: "CreatePhoto",
	172: "UpdateSubChunkBlocks",
	173: "PhotoInfoRequest",
	174: "SubChunk",
	175: "SubChunkRequest",
	176: "ClientStartItemCooldown",
	177: "ScriptMessage",
	178: "CodeBuilderSource",
	179: "TickingAreasLoadStatus",
	180: "DimensionData",
	181: "AgentAction",
	182: "ChangeMobProperty",
	183: "LessonProgress",
	184: "RequestAbility",
	185: "RequestPermissions",
	186: "ToastRequest",
	187: "UpdateAbilities",
	188: "UpdateAdventureSettings",
	189: "DeathInfo",
	190: "EditorNetwork",
	191: "FeatureRegistry",
	192: "ServerStats",
	193: "RequestNetworkSettings",
	194: "GameTestRequest",
	195: "GameTestResults",
	196: "UpdateClientInputLocks",
	197: "ClientCheatAbility",
	198: "CameraPresets",
	199: "UnlockedRecipes",
	200: "PyRpc",
	203: "NeteaseJson",
}

// raknetDial 用 sandertv/go-raknet 连接目标服务器（兼容 BDS/Geyser/cloudburst）。
// 强制 10 秒超时，避免 go-raknet 无限重试。
//
// 返回具体类型而非 net.Conn：调用方需要用 ReadPacket()（见读取循环的说明）。
func raknetDial(ctx context.Context, target string) (*gorsk.Conn, error) {
	dctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return gorsk.DialContext(dctx, target)
}

var (
	relayLogMu   sync.Mutex
	relayLogFile *os.File
)

// relayLogLine 把完整 hex 写入 relay.log（避免 stdout 管道截断长行）。
func relayLogLine(tag string, data []byte) {
	relayLogMu.Lock()
	defer relayLogMu.Unlock()
	if relayLogFile == nil {
		f, err := os.OpenFile("relay.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return
		}
		relayLogFile = f
	}
	fmt.Fprintf(relayLogFile, "%s %d: %x\n", tag, len(data), data)
}

// connectTan 连接中转服务器并完成 TanLogin。
func connectTan(ctx context.Context, cred *room.CreateResult, uid uint32, name string) (*session, error) {
	conn, err := raknet.DialContext(ctx, cred.RaknetServerAddress)
	if err != nil {
		return nil, fmt.Errorf("raknet dial %s: %w", cred.RaknetServerAddress, err)
	}
	enc := packet.NewEncoder(conn)
	dec, err := packet.NewDecoder(conn)
	if err != nil {
		conn.Close()
		return nil, err
	}
	if err := writePacket(enc, &packet.TanLoginRequest{
		PlayerID:   uid,
		Rand:       cred.RaknetRand,
		AESRand:    cred.RaknetAESRand,
		PlayerName: name,
	}); err != nil {
		conn.Close()
		return nil, err
	}
	pk, err := readPacket(dec)
	if err != nil {
		conn.Close()
		return nil, err
	}
	loginResp, ok := pk.(*packet.TanLoginResponse)
	if !ok {
		conn.Close()
		return nil, fmt.Errorf("unexpected login response: %T", pk)
	}
	if loginResp.ErrorCode != packet.TanLoginSuccess {
		conn.Close()
		return nil, fmt.Errorf("tan login failed: code=%d", loginResp.ErrorCode)
	}
	if err := enc.EnableEncryption(cred.EncryptKeyBytes, cred.DecryptKeyBytes); err != nil {
		conn.Close()
		return nil, err
	}
	if err := dec.EnableEncryption(cred.EncryptKeyBytes, cred.DecryptKeyBytes); err != nil {
		conn.Close()
		return nil, err
	}
	return &session{raknetConn: conn, enc: enc, dec: dec}, nil
}

func writePacket(encoder *packet.Encoder, pk packet.Packet) error {
	buf := bytes.NewBuffer(nil)
	writer := encoding.NewWriter(buf, 0)
	pk.Marshal(writer)
	header := packet.Header{PacketID: pk.ID()}
	headerBuf := bytes.NewBuffer(nil)
	if err := header.Write(headerBuf); err != nil {
		return err
	}
	full := append(headerBuf.Bytes(), buf.Bytes()...)
	return encoder.Encode(full)
}

func readPacket(decoder *packet.Decoder) (pk packet.Packet, err error) {
	pkData, err := decoder.Decode()
	if err != nil {
		return nil, err
	}
	// 这个协议的 decoder 在数据不够时是直接 panic（reader 里没有边界检查），
	// 而我们对部分包的线格式仍是推断出来的。这里兜住 panic，避免一个没解析
	// 成功的包把整个网关带崩 —— 并把原始字节打出来，便于事后核对真实格式。
	defer func() {
		if r := recover(); r != nil {
			pk = nil
			err = fmt.Errorf("解析包失败(%v) 原始数据=%x", r, pkData)
		}
	}()
	buf := bytes.NewBuffer(pkData)
	reader := encoding.NewReader(buf)
	header := packet.Header{}
	if err := header.Read(buf); err != nil {
		return nil, err
	}
	p := packet.NewServerPool()[header.PacketID]
	if p == nil {
		return nil, fmt.Errorf("未知包 ID: %d (数据=%x)", header.PacketID, pkData)
	}
	p.Marshal(reader)
	return p, nil
}

func randUint64() (uint64, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return 0, err
	}
	return uint64(b[0])<<56 | uint64(b[1])<<48 | uint64(b[2])<<40 | uint64(b[3])<<32 |
		uint64(b[4])<<24 | uint64(b[5])<<16 | uint64(b[6])<<8 | uint64(b[7]), nil
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "错误: "+format+"\n", args...)
	os.Exit(1)
}

// debugLogger 输出 pion 各子系统（ICE/DTLS/SCTP）的日志到 stderr。
// Debug/Info 仅在 -d / --debug 下输出（逐候选、逐 SACK，非常吵）；Warn/Error 始终输出。
type debugLogger struct{}

func (debugLogger) Trace(msg string)      {}
func (debugLogger) Tracef(string, ...any) {}
func (debugLogger) Debug(msg string) {
	if debugLog {
		log.Printf("[pion] %s", msg)
	}
}
func (debugLogger) Debugf(f string, a ...any) {
	if debugLog {
		log.Printf("[pion] "+f, a...)
	}
}
func (debugLogger) Info(msg string) {
	if debugLog {
		log.Printf("[pion] %s", msg)
	}
}
func (debugLogger) Infof(f string, a ...any) {
	if debugLog {
		log.Printf("[pion] "+f, a...)
	}
}
func (debugLogger) Warn(msg string)           { log.Printf("[pion] %s", msg) }
func (debugLogger) Warnf(f string, a ...any)  { log.Printf("[pion] "+f, a...) }
func (debugLogger) Error(msg string)          { log.Printf("[pion] %s", msg) }
func (debugLogger) Errorf(f string, a ...any) { log.Printf("[pion] "+f, a...) }

// netherLogger 是给 NetherNet 子模块（nemc-tan-lobby-solver）用的 logger。
// 该子模块的 Info 级日志是逐包 hex（ReliableDataChannel raw message 等），默认只放行
// Warn 及以上；加 -d / --debug 后全部放行。
func netherLogger() *slog.Logger {
	level := slog.LevelWarn
	if debugLog {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
}

type debugLoggerFactory struct{}

func (debugLoggerFactory) NewLogger(string) logging.LeveledLogger { return debugLogger{} }

/* ============================================================================
   以下是 Web 控制台部分（原先在 console.go —— 为了「单文件程序」合并进来）
   ============================================================================ */

// embeddedWeb 把前端打进二进制 —— 部署时只需要一个可执行文件。
//
//go:embed all:webui
var embeddedWeb embed.FS

/* ============================ 持久化数据结构 ============================ */

const consoleStateFile = "nbg-console.json"

type consoleSettings struct {
	Username     string `json:"username"`
	PasswordSalt string `json:"passwordSalt"`
	PasswordHash string `json:"passwordHash"`
	// PasswordIter 是 PBKDF2 迭代次数（老配置为 0，按默认值处理）
	PasswordIter int  `json:"passwordIter,omitempty"`
	PublicAccess bool `json:"publicAccess"`
}

type consoleAccount struct {
	ID      string `json:"id"`
	Note    string `json:"note"`
	Account string `json:"account"`
	// PasswordEnc 是 AES-256-GCM 加密后的 4399 密码（base64(nonce||密文)）
	PasswordEnc string `json:"passwordEnc,omitempty"`
	// Password 仅用于兼容老配置里的明文，迁移后清空
	Password string `json:"password,omitempty"`
}

type consoleCommand struct {
	ID      string `json:"id"`
	Trigger string `json:"trigger"`
	Line    string `json:"line"`
	Note    string `json:"note"`
	Enabled bool   `json:"enabled"`
}

type consoleInstance struct {
	ID           string           `json:"id"`
	Name         string           `json:"name"`
	AccountID    string           `json:"accountId"`
	Target       string           `json:"target"`
	RoomPassword string           `json:"roomPassword"`
	Capacity     int              `json:"capacity"`
	Commands     []consoleCommand `json:"commands"`
}

// publicAccount 是返回给前端的账号视图：只给账号名与掩码密码，
// 绝不下发明文、密文或加密密钥。
type publicAccount struct {
	ID           string `json:"id"`
	Note         string `json:"note"`
	Account      string `json:"account"`
	PasswordMask string `json:"passwordMask"`
}

// maskSecret 只保留前 2 位，其余打点；不足 3 位全部打点。
func maskSecret(s string) string {
	r := []rune(s)
	if len(r) <= 2 {
		return strings.Repeat("•", len(r))
	}
	n := len(r) - 2
	if n > 8 {
		n = 8
	}
	return string(r[:2]) + strings.Repeat("•", n)
}

// accountView 生成对外视图（能解密时用真实长度做掩码，否则退回长度未知的固定掩码）。
func (c *console) accountView(a consoleAccount) publicAccount {
	mask := ""
	if plain, err := c.accountSecret(a); err == nil && plain != "" {
		mask = maskSecret(plain)
	} else if a.Password != "" {
		mask = maskSecret(a.Password)
	} else {
		mask = "••••••"
	}
	return publicAccount{ID: a.ID, Note: a.Note, Account: a.Account, PasswordMask: mask}
}

type consoleState struct {
	Settings  consoleSettings   `json:"settings"`
	Accounts  []consoleAccount  `json:"accounts"`
	Instances []consoleInstance `json:"instances"`
}

/* ============================ 运行期状态 ============================ */

type logLine struct {
	Time  string `json:"time"`
	Level string `json:"level"`
	Msg   string `json:"msg"`
}

const maxLogLines = 800

// instanceRuntime 是一个实例的运行期状态（进程 + 日志环形缓冲 + 订阅者）。
type instanceRuntime struct {
	id string

	mu        sync.Mutex
	proc      *exec.Cmd
	status    string // stopped / running / error
	errMsg    string
	roomID    string
	players   int
	startedAt time.Time
	logs      []logLine
	subs      map[chan logLine]struct{}
	cfgPath   string
	roomFile  string
	lastRoom  string // 上一次的房间号，用于判断「房间号变化」
	recreated int
}

type console struct {
	mu    sync.Mutex
	state consoleState
	file  string

	sessMu   sync.Mutex
	sessions map[string]time.Time

	rtMu sync.Mutex
	rt   map[string]*instanceRuntime

	webRoot string
	dataDir string

	// key 是登录后用控制台密码派生出的加密密钥，只驻留内存；退出登录/关闭进程时清零
	key []byte

	// 登录失败限速：同一来源连续失败 5 次锁 30 秒
	failMu sync.Mutex
	fails  map[string]*loginFail
}

type loginFail struct {
	Count int
	Until time.Time
}

/* ============================ 启动入口 ============================ */

// serveConsole 启动 Web 控制台（阻塞）。
func serveConsole(port int, webRoot string) {
	if port < 1 || port > 65535 {
		fatalf("Web 控制台端口非法: %d（应为 1-65535）。"+
			"\n提示：4399 密码参数已改名为 -pass（原来的 -p 现在表示 Web 端口）", port)
	}

	c := &console{
		file:     consoleStateFile,
		sessions: map[string]time.Time{},
		rt:       map[string]*instanceRuntime{},
		fails:    map[string]*loginFail{},
		webRoot:  webRoot,
	}
	if err := c.load(); err != nil {
		log.Printf("[控制台] 读取 %s 失败（将按默认配置启动）: %v", c.file, err)
	}
	if c.state.Settings.Username == "" {
		c.state.Settings.Username = "user"
		c.setPassword("password") // 默认账号密码 user / password
	}
	if err := c.save(); err != nil {
		log.Printf("[控制台] 保存 %s 失败: %v", c.file, err)
	}

	mux := http.NewServeMux()
	c.routes(mux)

	binds := []string{fmt.Sprintf("127.0.0.1:%d", port)}
	if c.state.Settings.PublicAccess {
		binds = []string{fmt.Sprintf("0.0.0.0:%d", port), fmt.Sprintf("[::]:%d", port)}
	}

	log.Printf("[控制台] Web 控制台已启动：%s（%s）", strings.Join(binds, " , "),
		cond(c.state.Settings.PublicAccess, "公网可访问", "仅本机"))
	log.Printf("[控制台] 浏览 http://127.0.0.1:%d/ 登录（默认 user / password）", port)
	if c.webRoot != "" {
		log.Printf("[控制台] 前端：磁盘目录 %s（-web-root 调试模式）", c.webRoot)
	} else {
		log.Printf("[控制台] 前端：已内嵌在可执行文件里（单文件部署）")
	}

	// 公网访问时同时监听 IPv4/IPv6 通配地址，任一失败都不致命。
	var wg sync.WaitGroup
	errCh := make(chan error, len(binds))
	for _, b := range binds {
		ln, err := net.Listen("tcp", b)
		if err != nil {
			errCh <- fmt.Errorf("监听 %s 失败: %w", b, err)
			continue
		}
		wg.Add(1)
		srv := &http.Server{Handler: withSecurityHeaders(mux), ReadHeaderTimeout: 10 * time.Second}
		go func() {
			defer wg.Done()
			if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- err
			}
		}()
	}
	select {
	case err := <-errCh:
		if err != nil {
			fatalf("%v", err)
		}
	case <-time.After(300 * time.Millisecond):
	}
	wg.Wait()
}

func cond(b bool, t, f string) string {
	if b {
		return t
	}
	return f
}

/* ============================ 读写配置 ============================ */

func (c *console) load() error {
	b, err := os.ReadFile(c.file)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, &c.state)
}

// save 必须在持有 c.mu 时调用（或调用方自己保证串行）。
func (c *console) save() error {
	b, err := json.MarshalIndent(c.state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.file, append(b, '\n'), 0o600) // 内含 4399 明文密码 → 收紧权限
}

func (c *console) saveLocked() {
	if err := c.save(); err != nil {
		log.Printf("[控制台] 保存配置失败: %v", err)
	}
}

/* ============================ 口令与凭据加密 ============================ */
//
// 控制台密码是唯一口令来源（开房必须先登录控制台）：
//   kdf   = PBKDF2-HMAC-SHA256(密码, salt, iter, 32)
//   verif = HMAC-SHA256(kdf, "verify")   ← 存盘，用来验证密码
//   key   = HMAC-SHA256(kdf, "enc")      ← 不存盘，仅登录后驻留内存
// 4399 账号密码用 AES-256-GCM 加密后存盘；用时在内存解密，退出登录即清零 key。

const kdfIterations = 200000

// pbkdf2SHA256 是标准 PBKDF2（自己实现，避免引入 x/crypto 依赖）。
func pbkdf2SHA256(password, salt []byte, iter, keyLen int) []byte {
	h := hmac.New(sha256.New, password)
	hashLen := h.Size()
	blocks := (keyLen + hashLen - 1) / hashLen
	out := make([]byte, 0, blocks*hashLen)
	blk := make([]byte, 4)
	for b := 1; b <= blocks; b++ {
		h.Reset()
		h.Write(salt)
		blk[0], blk[1], blk[2], blk[3] = byte(b>>24), byte(b>>16), byte(b>>8), byte(b)
		h.Write(blk)
		u := h.Sum(nil)
		t := append([]byte(nil), u...)
		for i := 1; i < iter; i++ {
			h.Reset()
			h.Write(u)
			u = h.Sum(u[:0])
			for j := range t {
				t[j] ^= u[j]
			}
		}
		out = append(out, t...)
	}
	return out[:keyLen]
}

func hmacKey(secret []byte, label string) []byte {
	m := hmac.New(sha256.New, secret)
	m.Write([]byte(label))
	return m.Sum(nil)
}

// deriveKeys 由控制台密码推导校验值与加密密钥。
func deriveKeys(salt string, iter int, pass string) (verifier string, key []byte) {
	if iter <= 0 {
		iter = kdfIterations
	}
	kdf := pbkdf2SHA256([]byte(pass), []byte(salt), iter, 32)
	verifier = hex.EncodeToString(hmacKey(kdf, "verify"))
	key = hmacKey(kdf, "enc")
	wipe(kdf)
	return verifier, key
}

// wipe 清零缓冲区（Go 的 string 不可变，所以敏感数据尽量走 []byte）。
func wipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// setPassword 设置/更换控制台密码：重算 salt+迭代次数，并把新 key 放进内存。
func (c *console) setPassword(pass string) {
	salt := randHex(16)
	iter := kdfIterations
	verifier, key := deriveKeys(salt, iter, pass)
	c.state.Settings.PasswordSalt = salt
	c.state.Settings.PasswordIter = iter
	c.state.Settings.PasswordHash = verifier
	wipe(c.key)
	c.key = key
}

// checkPassword 校验密码（不改变内存里的 key）。
func (c *console) checkPassword(pass string) bool {
	iter := c.state.Settings.PasswordIter
	if iter <= 0 {
		iter = kdfIterations
	}
	verifier, key := deriveKeys(c.state.Settings.PasswordSalt, iter, pass)
	wipe(key)
	return subtle.ConstantTimeCompare([]byte(c.state.Settings.PasswordHash), []byte(verifier)) == 1
}

// encryptSecret 用内存中的 key 加密（AES-256-GCM），返回 base64(nonce||密文)。
func (c *console) encryptSecret(plain string) (string, error) {
	if len(c.key) != 32 {
		return "", errors.New("未登录：加密密钥不可用")
	}
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nil, nonce, []byte(plain), nil)
	return base64.StdEncoding.EncodeToString(append(nonce, sealed...)), nil
}

// decryptSecret 解密；key 不可用（未登录）时返回错误。
func (c *console) decryptSecret(enc string) (string, error) {
	if len(c.key) != 32 {
		return "", errors.New("未登录：加密密钥不可用")
	}
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return "", err
	}
	defer wipe(raw)
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("密文损坏")
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	if err != nil {
		return "", err
	}
	out := string(plain)
	wipe(plain)
	return out, nil
}

// accountSecret 取出账号的 4399 密码：优先解密 passwordEnc，兼容老配置的明文。
func (c *console) accountSecret(a consoleAccount) (string, error) {
	if a.PasswordEnc != "" {
		return c.decryptSecret(a.PasswordEnc)
	}
	return a.Password, nil
}

// migrateAccountsLocked 把老配置里明文保存的密码就地加密（需持有 c.mu 且已登录）。
func (c *console) migrateAccountsLocked() {
	for i := range c.state.Accounts {
		if c.state.Accounts[i].PasswordEnc == "" && c.state.Accounts[i].Password != "" {
			if enc, err := c.encryptSecret(c.state.Accounts[i].Password); err == nil {
				c.state.Accounts[i].PasswordEnc = enc
				c.state.Accounts[i].Password = ""
			}
		}
	}
}

func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(b)
}

func newID(prefix string) string { return prefix + randHex(5) }

/* ============================ 会话 ============================ */

const sessionCookie = "nbg_session"

// 会话只存在内存里，且 Cookie 不带 Max-Age → 关闭浏览器即失效。
func (c *console) newSession() string {
	tok := randHex(16)
	c.sessMu.Lock()
	c.sessions[tok] = time.Now()
	c.sessMu.Unlock()
	return tok
}

func (c *console) validSession(r *http.Request) bool {
	ck, err := r.Cookie(sessionCookie)
	if err != nil || ck.Value == "" {
		return false
	}
	c.sessMu.Lock()
	created, ok := c.sessions[ck.Value]
	if ok && time.Since(created) > 8*time.Hour { // 会话空闲上限 8 小时
		delete(c.sessions, ck.Value)
		ok = false
	}
	c.sessMu.Unlock()
	return ok
}

func (c *console) dropSession(r *http.Request) {
	if ck, err := r.Cookie(sessionCookie); err == nil {
		c.sessMu.Lock()
		delete(c.sessions, ck.Value)
		c.sessMu.Unlock()
	}
}

/* ============================ HTTP 路由 ============================ */

func (c *console) routes(mux *http.ServeMux) {
	// 无需登录
	mux.HandleFunc("/api/login", c.handleLogin)
	mux.HandleFunc("/api/session", c.handleSession)

	// 需要登录
	// 退出登录不要求会话有效：重复点、会话已过期都应当成功（幂等），否则前端会弹莫名其妙的错误
	mux.HandleFunc("/api/logout", c.handleLogout)
	mux.HandleFunc("/api/accounts", c.auth(c.handleAccounts))
	mux.HandleFunc("/api/accounts/", c.auth(c.handleAccountOne))
	mux.HandleFunc("/api/instances", c.auth(c.handleInstances))
	mux.HandleFunc("/api/instances/", c.auth(c.handleInstanceSub))
	mux.HandleFunc("/api/settings", c.auth(c.handleSettings))

	// 静态页面（未登录时页面自己会跳登录页）
	mux.HandleFunc("/", c.handleStatic)
}

// withSecurityHeaders 给所有响应加基础安全头。
func withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

func (c *console) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !c.validSession(r) {
			writeJSON(w, 401, map[string]any{"ok": false, "msg": "未登录或会话已过期"})
			return
		}
		next(w, r)
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func ok(w http.ResponseWriter, v any) { writeJSON(w, 200, v) }

func fail(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"ok": false, "msg": msg})
}

func decodeBody(r *http.Request, v any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	return dec.Decode(v)
}

func idFromPath(p, prefix string) string {
	s := strings.TrimPrefix(p, prefix)
	if i := strings.IndexByte(s, '/'); i >= 0 {
		return s[:i]
	}
	return s
}

/* ============================ 登录 / 会话 ============================ */

func (c *console) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		fail(w, 405, "只支持 POST")
		return
	}
	var in struct{ Username, Password string }
	if err := decodeBody(r, &in); err != nil {
		fail(w, 400, "请求体解析失败")
		return
	}
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	c.failMu.Lock()
	f := c.fails[ip]
	if f != nil && time.Now().Before(f.Until) {
		wait := int(time.Until(f.Until).Seconds()) + 1
		c.failMu.Unlock()
		fail(w, 429, fmt.Sprintf("登录失败次数过多，请 %d 秒后再试", wait))
		return
	}
	c.failMu.Unlock()

	c.mu.Lock()
	userOK := subtle.ConstantTimeCompare([]byte(in.Username), []byte(c.state.Settings.Username)) == 1
	iter := c.state.Settings.PasswordIter
	if iter <= 0 {
		iter = kdfIterations
	}
	verifier, key := deriveKeys(c.state.Settings.PasswordSalt, iter, in.Password)
	passOK := subtle.ConstantTimeCompare([]byte(c.state.Settings.PasswordHash), []byte(verifier)) == 1
	if passOK {
		wipe(c.key) // 销毁上一个 key
		c.key = key // 本次会话的加密密钥，只驻留内存
		c.migrateAccountsLocked()
		c.saveLocked()
	} else {
		wipe(key)
	}
	c.mu.Unlock()

	if !userOK || !passOK {
		c.failMu.Lock()
		rec := c.fails[ip]
		if rec == nil {
			rec = &loginFail{}
			c.fails[ip] = rec
		}
		rec.Count++
		if rec.Count >= 5 {
			rec.Until = time.Now().Add(30 * time.Second)
			rec.Count = 0
		}
		c.failMu.Unlock()
		time.Sleep(400 * time.Millisecond) // 轻微拖慢暴力尝试
		fail(w, 401, "用户名或密码错误")
		return
	}
	c.failMu.Lock()
	delete(c.fails, ip)
	c.failMu.Unlock()
	tok := c.newSession()
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: tok, Path: "/",
		HttpOnly: true, SameSite: http.SameSiteLaxMode,
		// 不设 Max-Age / Expires → 浏览器会话 Cookie，关闭浏览器即删除
	})
	ok(w, map[string]any{"ok": true})
}

func (c *console) handleLogout(w http.ResponseWriter, r *http.Request) {
	c.dropSession(r)
	c.mu.Lock()
	wipe(c.key) // 明文用完即销毁：退出登录后无法再解密任何账号密码
	c.key = nil
	c.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
	ok(w, map[string]any{"ok": true})
}

func (c *console) handleSession(w http.ResponseWriter, r *http.Request) {
	ok(w, map[string]any{"ok": true, "loggedIn": c.validSession(r)})
}

/* ============================ 账号 ============================ */

func (c *console) handleAccounts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		c.mu.Lock()
		out := make([]publicAccount, 0, len(c.state.Accounts))
		for _, a := range c.state.Accounts {
			out = append(out, c.accountView(a))
		}
		c.mu.Unlock()
		ok(w, out)
	case http.MethodPost:
		var a consoleAccount
		if err := decodeBody(r, &a); err != nil {
			fail(w, 400, "请求体解析失败")
			return
		}
		if a.Account == "" || a.Password == "" {
			fail(w, 400, "账号和密码都要填")
			return
		}
		enc, err := c.encryptSecret(a.Password)
		if err != nil {
			fail(w, 400, err.Error())
			return
		}
		a.PasswordEnc, a.Password = enc, ""
		a.ID = newID("acc")
		c.mu.Lock()
		c.state.Accounts = append(c.state.Accounts, a)
		c.saveLocked()
		view := c.accountView(a)
		c.mu.Unlock()
		ok(w, view)
	default:
		fail(w, 405, "不支持的方法")
	}
}

func (c *console) handleAccountOne(w http.ResponseWriter, r *http.Request) {
	id := idFromPath(r.URL.Path, "/api/accounts/")
	if id == "" {
		fail(w, 400, "缺少账号 id")
		return
	}
	switch r.Method {
	case http.MethodPut:
		var in consoleAccount
		if err := decodeBody(r, &in); err != nil {
			fail(w, 400, "请求体解析失败")
			return
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		for i := range c.state.Accounts {
			if c.state.Accounts[i].ID == id {
				if in.Account != "" {
					c.state.Accounts[i].Account = in.Account
				}
				if in.Password != "" {
					enc, err := c.encryptSecret(in.Password)
					if err != nil {
						fail(w, 400, err.Error())
						return
					}
					c.state.Accounts[i].PasswordEnc, c.state.Accounts[i].Password = enc, ""
				}
				c.state.Accounts[i].Note = in.Note
				c.saveLocked()
				view := c.accountView(c.state.Accounts[i])
				ok(w, view)
				return
			}
		}
		fail(w, 404, "账号不存在")
	case http.MethodDelete:
		c.mu.Lock()
		defer c.mu.Unlock()
		for _, inst := range c.state.Instances {
			if inst.AccountID == id {
				fail(w, 409, "账号已被实例「"+inst.Name+"」使用，请先改掉")
				return
			}
		}
		out := c.state.Accounts[:0]
		found := false
		for _, a := range c.state.Accounts {
			if a.ID == id {
				found = true
				continue
			}
			out = append(out, a)
		}
		if !found {
			fail(w, 404, "账号不存在")
			return
		}
		c.state.Accounts = out
		c.saveLocked()
		ok(w, map[string]any{"ok": true})
	default:
		fail(w, 405, "不支持的方法")
	}
}

/* ============================ 实例 ============================ */

func (c *console) handleInstances(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		c.mu.Lock()
		list := append([]consoleInstance(nil), c.state.Instances...)
		c.mu.Unlock()
		out := make([]map[string]any, 0, len(list))
		for _, inst := range list {
			out = append(out, c.instanceView(inst))
		}
		ok(w, out)
	case http.MethodPost:
		var in consoleInstance
		if err := decodeBody(r, &in); err != nil {
			fail(w, 400, "请求体解析失败")
			return
		}
		if msg := c.validateInstance(in, ""); msg != "" {
			fail(w, 400, msg)
			return
		}
		in.ID = newID("inst")
		if in.Capacity <= 0 {
			in.Capacity = 8
		}
		c.mu.Lock()
		c.state.Instances = append(c.state.Instances, in)
		c.saveLocked()
		c.mu.Unlock()
		c.rtFor(in.ID) // 建好运行期槽位（状态 stopped）
		ok(w, in)
	default:
		fail(w, 405, "不支持的方法")
	}
}

// validateInstance 校验实例字段；一个账号至多绑定一个实例。
func (c *console) validateInstance(in consoleInstance, selfID string) string {
	if strings.TrimSpace(in.AccountID) == "" {
		return "请先选择账号"
	}
	if !regexp.MustCompile(`^.+:\d{1,5}$`).MatchString(strings.TrimSpace(in.Target)) {
		return "服务器地址要写成 IP:端口"
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	found := false
	for _, a := range c.state.Accounts {
		if a.ID == in.AccountID {
			found = true
			break
		}
	}
	if !found {
		return "账号不存在"
	}
	for _, inst := range c.state.Instances {
		if inst.AccountID == in.AccountID && inst.ID != selfID {
			return "该账号已被实例「" + inst.Name + "」使用（一个账号只能对应一个实例）"
		}
	}
	return ""
}

func (c *console) handleInstanceSub(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/instances/")
	parts := strings.SplitN(rest, "/", 2)
	id := parts[0]
	sub := ""
	if len(parts) > 1 {
		sub = parts[1]
	}
	if id == "" {
		fail(w, 400, "缺少实例 id")
		return
	}

	switch {
	case sub == "" && r.Method == http.MethodGet:
		c.mu.Lock()
		inst, found := c.findInstance(id)
		c.mu.Unlock()
		if !found {
			fail(w, 404, "实例不存在")
			return
		}
		ok(w, c.instanceView(inst))
	case sub == "" && r.Method == http.MethodPut:
		var in consoleInstance
		if err := decodeBody(r, &in); err != nil {
			fail(w, 400, "请求体解析失败")
			return
		}
		if msg := c.validateInstance(in, id); msg != "" {
			fail(w, 400, msg)
			return
		}
		c.mu.Lock()
		for i := range c.state.Instances {
			if c.state.Instances[i].ID == id {
				in.ID = id
				if in.Capacity <= 0 {
					in.Capacity = 8
				}
				if in.Commands == nil {
					in.Commands = c.state.Instances[i].Commands
				}
				c.state.Instances[i] = in
				c.saveLocked()
				c.mu.Unlock()
				ok(w, in)
				return
			}
		}
		c.mu.Unlock()
		fail(w, 404, "实例不存在")
	case sub == "" && r.Method == http.MethodDelete:
		c.rtMu.Lock()
		if rt := c.rt[id]; rt != nil {
			rt.mu.Lock()
			running := rt.proc != nil
			rt.mu.Unlock()
			if running {
				c.rtMu.Unlock()
				fail(w, 409, "实例正在运行，请先停止")
				return
			}
			delete(c.rt, id)
		}
		c.rtMu.Unlock()
		c.mu.Lock()
		defer c.mu.Unlock()
		out := c.state.Instances[:0]
		found := false
		for _, inst := range c.state.Instances {
			if inst.ID == id {
				found = true
				continue
			}
			out = append(out, inst)
		}
		if !found {
			fail(w, 404, "实例不存在")
			return
		}
		c.state.Instances = out
		c.saveLocked()
		ok(w, map[string]any{"ok": true})
	case sub == "start":
		c.startInstance(w, id)
	case sub == "stop":
		c.stopInstance(w, id)
	case sub == "status":
		ok(w, c.instanceStatus(id))
	case sub == "logs":
		c.streamLogs(w, r, id)
	case sub == "commands":
		c.handleCommands(w, r, id, "")
	case strings.HasPrefix(sub, "commands/"):
		c.handleCommands(w, r, id, strings.TrimPrefix(sub, "commands/"))
	default:
		fail(w, 404, "未知接口")
	}
}

func (c *console) findInstance(id string) (consoleInstance, bool) {
	for _, inst := range c.state.Instances {
		if inst.ID == id {
			return inst, true
		}
	}
	return consoleInstance{}, false
}

// instanceView 把实例与运行期状态合成前端需要的形状。
func (c *console) instanceView(inst consoleInstance) map[string]any {
	rt := c.rtFor(inst.ID)
	rt.mu.Lock()
	status, roomID, players, errMsg := rt.status, rt.roomID, rt.players, rt.errMsg
	rt.mu.Unlock()
	if status == "" {
		status = "stopped"
	}
	acc := ""
	c.mu.Lock()
	for _, a := range c.state.Accounts {
		if a.ID == inst.AccountID {
			acc = a.Account
			break
		}
	}
	c.mu.Unlock()
	return map[string]any{
		"id": inst.ID, "name": inst.Name, "accountId": inst.AccountID, "account": acc,
		"target": inst.Target, "roomPassword": inst.RoomPassword, "capacity": inst.Capacity,
		"commands": inst.Commands,
		"status":   status, "roomId": roomID, "players": players, "msg": errMsg,
	}
}

func (c *console) instanceStatus(id string) map[string]any {
	c.mu.Lock()
	inst, found := c.findInstance(id)
	c.mu.Unlock()
	if !found {
		return map[string]any{"status": "unknown"}
	}
	return c.instanceView(inst)
}

/* ============================ 实例进程 ============================ */

func (c *console) rtFor(id string) *instanceRuntime {
	c.rtMu.Lock()
	defer c.rtMu.Unlock()
	rt := c.rt[id]
	if rt == nil {
		rt = &instanceRuntime{id: id, status: "stopped", subs: map[chan logLine]struct{}{}}
		c.rt[id] = rt
	}
	return rt
}

// 实例配置文件：交给子进程的参数（避免密码出现在命令行里）
type runConfigFile struct {
	Username         string `json:"username"`
	Password         string `json:"password"`
	RoomName         string `json:"roomName"`
	Capacity         uint   `json:"capacity"`
	RoomPassword     string `json:"roomPassword"`
	Target           string `json:"target"`
	ServerAddress    string `json:"serverAddress"`
	MapID            uint64 `json:"mapId"`
	ProtocolID       uint   `json:"protocolId"`
	LevelID          string `json:"levelId"`
	GameType         uint   `json:"gameType"`
	VersionString    string `json:"versionString"`
	RoomFile         string `json:"roomFile"`
	KeepaliveSeconds int    `json:"keepaliveSeconds"`
	Debug            bool   `json:"debug"`
}

func (c *console) startInstance(w http.ResponseWriter, id string) {
	c.mu.Lock()
	inst, found := c.findInstance(id)
	var acc consoleAccount
	for _, a := range c.state.Accounts {
		if a.ID == inst.AccountID {
			acc = a
			break
		}
	}
	c.mu.Unlock()
	if !found {
		fail(w, 404, "实例不存在")
		return
	}
	if acc.ID == "" {
		fail(w, 400, "实例绑定的账号不存在，请先编辑实例选择账号")
		return
	}
	accSecret, derr := c.accountSecret(acc)
	if derr != nil {
		fail(w, 400, "无法取出账号密码："+derr.Error()+"（登录后加密密钥才可用）")
		return
	}

	rt := c.rtFor(id)
	rt.mu.Lock()
	if rt.proc != nil {
		rt.mu.Unlock()
		fail(w, 409, "实例已在运行")
		return
	}
	rt.mu.Unlock()

	self, err := os.Executable()
	if err != nil {
		fail(w, 500, "取可执行文件路径失败: "+err.Error())
		return
	}

	dir := c.instanceDir(id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		fail(w, 500, "建实例目录失败: "+err.Error())
		return
	}
	roomFile := filepath.Join(dir, "room.json")
	cfgPath := filepath.Join(dir, "instance.json")
	cfg := runConfigFile{
		Username: acc.Account, Password: accSecret,
		RoomName: inst.Name, Capacity: uint(inst.Capacity), RoomPassword: inst.RoomPassword,
		Target: inst.Target, ServerAddress: inst.Target,
		LevelID: defaultLevelID, VersionString: "1.21.120.0",
		ProtocolID: 42, RoomFile: roomFile,
		KeepaliveSeconds: 25, Debug: debugLog,
	}
	b, _ := json.MarshalIndent(cfg, "", "  ")
	if err := os.WriteFile(cfgPath, b, 0o600); err != nil {
		wipe(b)
		fail(w, 500, "写实例配置失败: "+err.Error())
		return
	}
	wipe(b) // 内存里的明文 JSON 立刻销毁（配置文件由子进程读完自删）

	cmd := exec.Command(self, "-run", cfgPath)
	cmd.Dir = dir
	pr, pw := io.Pipe()
	cmd.Stdout, cmd.Stderr = pw, pw
	if err := cmd.Start(); err != nil {
		_ = pw.Close()
		fail(w, 500, "启动实例进程失败: "+err.Error())
		return
	}

	rt.mu.Lock()
	rt.proc = cmd
	rt.status = "running"
	rt.errMsg = ""
	rt.roomID = ""
	rt.players = 0
	rt.cfgPath = cfgPath
	rt.roomFile = roomFile
	rt.startedAt = time.Now()
	rt.mu.Unlock()

	c.appendLog(id, "INFO", "[控制台] 实例进程已启动 pid="+strconv.Itoa(cmd.Process.Pid))
	go c.pumpLogs(rt, pr)
	go c.waitProcess(rt, pw)
	c.fireCommands(id, "instance.start")

	ok(w, c.instanceStatus(id))
}

func (c *console) stopInstance(w http.ResponseWriter, id string) {
	rt := c.rtFor(id)
	rt.mu.Lock()
	cmd := rt.proc
	rt.mu.Unlock()
	if cmd == nil {
		ok(w, c.instanceStatus(id))
		return
	}
	c.appendLog(id, "INFO", "[控制台] 正在停止实例 ...")
	killProcess(cmd)
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		rt.mu.Lock()
		alive := rt.proc != nil
		rt.mu.Unlock()
		if !alive {
			break
		}
		time.Sleep(120 * time.Millisecond)
	}
	c.fireCommands(id, "instance.stop")
	ok(w, c.instanceStatus(id))
}

func killProcess(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	if runtime.GOOS == "windows" {
		// 连同可能拉起的子进程一起结束
		_ = exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
		return
	}
	_ = cmd.Process.Signal(os.Interrupt)
	time.Sleep(300 * time.Millisecond)
	_ = cmd.Process.Kill()
}

// waitProcess 等子进程退出并更新状态。
func (c *console) waitProcess(rt *instanceRuntime, pw *io.PipeWriter) {
	err := rt.proc.Wait()
	_ = pw.Close()
	rt.mu.Lock()
	rt.proc = nil
	if err != nil {
		rt.status = "error"
		rt.errMsg = "进程退出: " + err.Error()
	} else {
		rt.status = "stopped"
		rt.errMsg = ""
	}
	rt.roomID = ""
	rt.players = 0
	rt.mu.Unlock()
	c.appendLog(rt.id, "INFO", "[控制台] 实例进程已退出")
}

/* ============================ 日志：捕获 / 解析 / 推送 ============================ */

func (c *console) appendLog(id, level, msg string) {
	line := logLine{Time: time.Now().Format("15:04:05"), Level: level, Msg: msg}
	rt := c.rtFor(id)
	rt.mu.Lock()
	rt.logs = append(rt.logs, line)
	if len(rt.logs) > maxLogLines {
		rt.logs = rt.logs[len(rt.logs)-maxLogLines:]
	}
	subs := make([]chan logLine, 0, len(rt.subs))
	for ch := range rt.subs {
		subs = append(subs, ch)
	}
	rt.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- line:
		default: // 订阅者太慢就丢这一行，不阻塞
		}
	}
}

func (c *console) pumpLogs(rt *instanceRuntime, pr *io.PipeReader) {
	sc := bufio.NewScanner(pr)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		c.appendLog(rt.id, classifyLevel(line), line)
		c.observeLine(rt, line)
	}
}

// classifyLevel 粗略判断日志级别（子进程输出是 slog 文本格式）。
func classifyLevel(line string) string {
	switch {
	case strings.Contains(line, "level=ERROR") || strings.Contains(line, "错误") ||
		strings.Contains(line, "panic") || strings.Contains(line, "失败:"):
		return "ERROR"
	case strings.Contains(line, "level=WARN") || strings.Contains(line, "警告"):
		return "WARN"
	case strings.Contains(line, "level=DEBUG"):
		return "DEBUG"
	default:
		return "INFO"
	}
}

var (
	reRoomCreated = regexp.MustCompile(`RoomID=(\d+)`)
	reAlive       = regexp.MustCompile(`房间 (\d+) 存活（在线 (\d+) 人`)
	reLoginOK     = regexp.MustCompile(`认证成功`)
	reJoin        = regexp.MustCompile(`★ 新玩家加入房间`)
	reLeave       = regexp.MustCompile(`\[转发\] 玩家连接结束`)
	reRoomDead    = regexp.MustCompile(`房间 (\d+) 失效`)
	reKeepFail    = regexp.MustCompile(`房间存活检查失败`)
)

// observeLine 从子进程日志里提取事件，更新状态并触发命令。
func (c *console) observeLine(rt *instanceRuntime, line string) {
	rt.mu.Lock()
	switch {
	case reAlive.MatchString(line):
		m := reAlive.FindStringSubmatch(line)
		rt.roomID = m[1]
		rt.players, _ = strconv.Atoi(m[2])
	case reRoomCreated.MatchString(line):
		m := reRoomCreated.FindStringSubmatch(line)
		if rt.roomID != "" && rt.roomID != m[1] {
			rt.recreated++
		}
		rt.roomID = m[1]
	case reLeave.MatchString(line):
		if rt.players > 0 {
			rt.players--
		}
	}
	rt.mu.Unlock()

	c.mu.Lock()
	inst, found := c.findInstance(rt.id)
	c.mu.Unlock()
	if !found {
		return
	}
	room := c.roomIDOf(rt)
	switch {
	case reLoginOK.MatchString(line):
		c.runTrigger(inst, "account.login", room)
	case reJoin.MatchString(line):
		c.runTrigger(inst, "player.join", room)
	case reLeave.MatchString(line):
		c.runTrigger(inst, "player.leave", room)
	case reKeepFail.MatchString(line):
		c.runTrigger(inst, "keepalive.fail", room)
	case reRoomCreated.MatchString(line):
		now := c.roomIDOf(rt)
		last := c.lastRoomOf(rt, now)
		if last == "" {
			c.runTrigger(inst, "room.created", now)
		} else {
			c.runTrigger(inst, "room.changed", now)
			c.runTrigger(inst, "room.recreated", now)
		}
	case reRoomDead.MatchString(line):
		// 房间失效 → 下一次 RoomID 出现即视为重建
	}
}

func (c *console) roomIDOf(rt *instanceRuntime) string {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.roomID
}

func (c *console) lastRoomOf(rt *instanceRuntime, cur string) string {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	last := rt.lastRoom
	rt.lastRoom = cur
	return last
}

/* ============================ 日志接口（含 SSE 实时） ============================ */

func (c *console) streamLogs(w http.ResponseWriter, r *http.Request, id string) {
	rt := c.rtFor(id)
	tail := 300
	if v := r.URL.Query().Get("tail"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= maxLogLines {
			tail = n
		}
	}
	rt.mu.Lock()
	logs := append([]logLine(nil), rt.logs...)
	rt.mu.Unlock()
	if len(logs) > tail {
		logs = logs[len(logs)-tail:]
	}

	if r.URL.Query().Get("stream") != "1" {
		ok(w, logs)
		return
	}

	flusher, canFlush := w.(http.Flusher)
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	for _, l := range logs {
		writeSSE(w, l)
	}
	if canFlush {
		flusher.Flush()
	}

	ch := make(chan logLine, 256)
	rt.mu.Lock()
	rt.subs[ch] = struct{}{}
	rt.mu.Unlock()
	defer func() {
		rt.mu.Lock()
		delete(rt.subs, ch)
		rt.mu.Unlock()
	}()

	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case l := <-ch:
			writeSSE(w, l)
			if canFlush {
				flusher.Flush()
			}
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			if canFlush {
				flusher.Flush()
			}
		}
	}
}

func writeSSE(w io.Writer, l logLine) {
	b, _ := json.Marshal(l)
	fmt.Fprintf(w, "data: %s\n\n", b)
}

/* ============================ 命令行（事件触发） ============================ */

func (c *console) handleCommands(w http.ResponseWriter, r *http.Request, instID, cmdID string) {
	switch {
	case cmdID == "" && r.Method == http.MethodGet:
		c.mu.Lock()
		inst, found := c.findInstance(instID)
		c.mu.Unlock()
		if !found {
			fail(w, 404, "实例不存在")
			return
		}
		ok(w, inst.Commands)
	case cmdID == "" && r.Method == http.MethodPost:
		var in consoleCommand
		if err := decodeBody(r, &in); err != nil {
			fail(w, 400, "请求体解析失败")
			return
		}
		if strings.TrimSpace(in.Line) == "" {
			fail(w, 400, "命令行不能为空")
			return
		}
		in.ID = newID("cmd")
		c.mu.Lock()
		defer c.mu.Unlock()
		for i := range c.state.Instances {
			if c.state.Instances[i].ID == instID {
				c.state.Instances[i].Commands = append(c.state.Instances[i].Commands, in)
				c.saveLocked()
				ok(w, in)
				return
			}
		}
		fail(w, 404, "实例不存在")
	case cmdID != "" && r.Method == http.MethodPut:
		var in consoleCommand
		if err := decodeBody(r, &in); err != nil {
			fail(w, 400, "请求体解析失败")
			return
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		for i := range c.state.Instances {
			if c.state.Instances[i].ID != instID {
				continue
			}
			for j := range c.state.Instances[i].Commands {
				if c.state.Instances[i].Commands[j].ID == cmdID {
					in.ID = cmdID
					c.state.Instances[i].Commands[j] = in
					c.saveLocked()
					ok(w, in)
					return
				}
			}
		}
		fail(w, 404, "命令不存在")
	case cmdID != "" && r.Method == http.MethodDelete:
		c.mu.Lock()
		defer c.mu.Unlock()
		for i := range c.state.Instances {
			if c.state.Instances[i].ID != instID {
				continue
			}
			cmds := c.state.Instances[i].Commands
			out := cmds[:0]
			found := false
			for _, cm := range cmds {
				if cm.ID == cmdID {
					found = true
					continue
				}
				out = append(out, cm)
			}
			if !found {
				fail(w, 404, "命令不存在")
				return
			}
			c.state.Instances[i].Commands = out
			c.saveLocked()
			ok(w, map[string]any{"ok": true})
			return
		}
		fail(w, 404, "实例不存在")
	default:
		fail(w, 405, "不支持的方法")
	}
}

// fireCommands 由控制台自身触发（启动/停止）。
func (c *console) fireCommands(instID, trigger string) {
	c.mu.Lock()
	inst, found := c.findInstance(instID)
	c.mu.Unlock()
	if !found {
		return
	}
	c.runTrigger(inst, trigger, c.roomIDOf(c.rtFor(instID)))
}

func (c *console) runTrigger(inst consoleInstance, trigger, roomID string) {
	var hits []consoleCommand
	for _, cm := range inst.Commands {
		if cm.Enabled && cm.Trigger == trigger {
			hits = append(hits, cm)
		}
	}
	if len(hits) == 0 {
		return
	}
	rt := c.rtFor(inst.ID)
	rt.mu.Lock()
	players := rt.players
	rt.mu.Unlock()

	for _, cm := range hits {
		line := c.expandPlaceholders(cm.Line, inst, roomID, players, trigger)
		c.appendLog(inst.ID, "INFO", "[命令] "+trigger+" → "+line)
		go c.execCommand(inst.ID, line)
	}
}

// expandPlaceholders 替换命令里的占位符（{&roomid}、{&timestamp} 等）。
func (c *console) expandPlaceholders(s string, inst consoleInstance, roomID string, players int, trigger string) string {
	acc := ""
	c.mu.Lock()
	for _, a := range c.state.Accounts {
		if a.ID == inst.AccountID {
			acc = a.Account
			break
		}
	}
	c.mu.Unlock()

	repl := map[string]string{
		"{&roomid}":       roomID,
		"{&timestamp}":    strconv.FormatInt(time.Now().Unix(), 10),
		"{&datetime}":     time.Now().Format("2006-01-02 15:04:05"),
		"{&account}":      acc,
		"{&instancename}": inst.Name,
		"{&server}":       inst.Target,
		"{&capacity}":     strconv.Itoa(inst.Capacity),
		"{&players}":      strconv.Itoa(players),
		"{&event}":        trigger,
	}
	keys := make([]string, 0, len(repl))
	for k := range repl {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) }) // 长的先替换
	for _, k := range keys {
		s = strings.ReplaceAll(s, k, repl[k])
	}
	return s
}

func (c *console) execCommand(instID, line string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/c", line)
	} else {
		cmd = exec.CommandContext(ctx, "/bin/sh", "-c", line)
	}
	out, err := cmd.CombinedOutput()
	txt := strings.TrimSpace(string(out))
	if txt != "" {
		for _, l := range strings.Split(txt, "\n") {
			c.appendLog(instID, "INFO", "[命令输出] "+l)
		}
	}
	if err != nil {
		c.appendLog(instID, "ERROR", "[命令失败] "+err.Error())
		return
	}
	c.appendLog(instID, "INFO", "[命令完成] 退出码 0")
}

/* ============================ 设置 ============================ */

func (c *console) handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		c.mu.Lock()
		s := c.state.Settings
		c.mu.Unlock()
		ok(w, map[string]any{"user": s.Username, "publicAccess": s.PublicAccess})
	case http.MethodPost:
		var in struct {
			User         string `json:"user"`
			Password     string `json:"password"`
			PublicAccess *bool  `json:"publicAccess"`
		}
		if err := decodeBody(r, &in); err != nil {
			fail(w, 400, "请求体解析失败")
			return
		}
		c.mu.Lock()
		if strings.TrimSpace(in.User) == "" {
			c.mu.Unlock()
			fail(w, 400, "用户名不能为空")
			return
		}
		c.state.Settings.Username = strings.TrimSpace(in.User)
		if in.Password != "" {
			// 先用旧 key 解出全部账号密码，换 key 后再全部重新加密
			plains := make([]string, len(c.state.Accounts))
			for i, a := range c.state.Accounts {
				plain, err := c.accountSecret(a)
				if err != nil {
					c.mu.Unlock()
					fail(w, 400, "更换密码失败：无法解密已有账号密码（"+err.Error()+"）")
					return
				}
				plains[i] = plain
			}
			c.setPassword(in.Password) // 新 key 已就位
			for i := range c.state.Accounts {
				if enc, err := c.encryptSecret(plains[i]); err == nil {
					c.state.Accounts[i].PasswordEnc = enc
					c.state.Accounts[i].Password = ""
				}
				plains[i] = ""
			}
		}
		if in.PublicAccess != nil {
			c.state.Settings.PublicAccess = *in.PublicAccess
		}
		c.saveLocked()
		pub := c.state.Settings.PublicAccess
		c.mu.Unlock()
		ok(w, map[string]any{"ok": true, "publicAccess": pub,
			"msg": cond(in.Password != "", "已保存；公网访问改动需重启网关生效", "已保存；公网访问改动需重启网关生效")})
	default:
		fail(w, 405, "不支持的方法")
	}
}

/* ============================ 静态文件 ============================ */

func (c *console) handleStatic(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		fail(w, 404, "未知接口")
		return
	}
	p := strings.TrimPrefix(r.URL.Path, "/")
	if p == "" {
		p = "login.html"
	}
	p = path.Clean("/" + p) // 用 path（正斜杠）而不是 filepath：内嵌 FS 的键是正斜杠

	// -web-root 给了目录就用磁盘上的（改前端不用重新编译，方便调试）
	if c.webRoot != "" {
		full := filepath.Join(c.webRoot, p)
		if st, err := os.Stat(full); err != nil || st.IsDir() {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, full)
		return
	}

	// 默认：直接用打进二进制里的前端
	data, err := embeddedWeb.ReadFile("webui" + p)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", mimeByExt(p))
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)
}

// mimeByExt 内嵌文件没有扩展名映射表，自己按后缀给 Content-Type。
func mimeByExt(p string) string {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".html":
		return "text/html; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".js":
		return "text/javascript; charset=utf-8"
	case ".json":
		return "application/json; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	case ".png":
		return "image/png"
	case ".ico":
		return "image/x-icon"
	default:
		return "application/octet-stream"
	}
}

// findWebRoot 找磁盘上的前端目录（开发用）。找不到返回 ""，表示用内嵌前端。
func findWebRoot(explicit string) string {
	cands := []string{}
	if explicit != "" {
		cands = append(cands, explicit)
	}
	if exe, err := os.Executable(); err == nil {
		cands = append(cands, filepath.Join(filepath.Dir(exe), "webui"), filepath.Join(filepath.Dir(exe), "web"))
	}
	if wd, err := os.Getwd(); err == nil {
		cands = append(cands, filepath.Join(wd, "webui"), filepath.Join(wd, "web"), filepath.Join(wd, "..", "webui"))
	}
	for _, d := range cands {
		if st, err := os.Stat(filepath.Join(d, "login.html")); err == nil && !st.IsDir() {
			if abs, err := filepath.Abs(d); err == nil {
				return abs
			}
			return d
		}
	}
	return "" // 没找到 → 用内嵌前端
}

/* ============================ 实例目录 ============================ */

func (c *console) instanceDir(id string) string {
	if c.dataDir == "" {
		if wd, err := os.Getwd(); err == nil {
			c.dataDir = filepath.Join(wd, "instances")
		} else {
			c.dataDir = "instances"
		}
	}
	return filepath.Join(c.dataDir, id)
}

/* ============================ 实例配置读取（-run 模式） ============================ */

// loadRunConfig 读取控制台为实例生成的 JSON 配置。
func loadRunConfig(path string) (*runConfigFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rc runConfigFile
	if err := json.Unmarshal(b, &rc); err != nil {
		wipe(b)
		return nil, err
	}
	wipe(b)
	_ = os.Remove(path) // 明文配置只在启动瞬间存在过，读完即删
	if rc.Username == "" || rc.Password == "" || rc.Target == "" {
		return nil, fmt.Errorf("实例配置缺少 username/password/target")
	}
	return &rc, nil
}

/* ============================ -install：注册 systemd 服务 ============================ */

func installSystemd(port int) {
	if runtime.GOOS != "linux" {
		fatalf("-install 只在 Linux 下可用（当前系统: %s）", runtime.GOOS)
	}
	exe, err := os.Executable()
	if err != nil {
		fatalf("取可执行文件路径失败: %v", err)
	}
	wd, _ := os.Getwd()
	unit := fmt.Sprintf(`[Unit]
Description=NeteaseBedrockGateway 控制台
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=%s
ExecStart=%s -p %d
Restart=always
RestartSec=10
StandardOutput=append:%s/host.log
StandardError=append:%s/host.log

[Install]
WantedBy=multi-user.target
`, wd, exe, port, wd, wd)

	const unitPath = "/etc/systemd/system/netease-gateway.service"
	if os.Geteuid() != 0 {
		fmt.Printf("需要 root 权限写入 %s。请以 root 执行下面两条命令：\n\n", unitPath)
		fmt.Printf("  sudo tee %s > /dev/null <<'EOF'\n%sEOF\n\n", unitPath, unit)
		fmt.Printf("  sudo systemctl daemon-reload && sudo systemctl enable --now netease-gateway\n")
		return
	}
	if err := os.WriteFile(unitPath, []byte(unit), 0o600); err != nil {
		fatalf("写入 %s 失败: %v", unitPath, err)
	}
	fmt.Printf("已写入 %s\n执行: systemctl daemon-reload && systemctl enable --now netease-gateway\n", unitPath)
}

/* ============================ 排序辅助（稳定输出，便于排查） ============================ */

func sortStrings(s []string) { sort.Strings(s) }
