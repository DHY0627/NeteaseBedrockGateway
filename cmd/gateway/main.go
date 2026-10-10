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
	"bytes"
	"compress/flate"
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"os/signal"
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
		password     = flag.String("p", "", "4399 密码")
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
	)
	flag.Parse()
	netherMsgLimit = *netherMsgLimitFlag
	debugLog = *debugShort || *debugLong
	if *username == "" || *password == "" || *target == "" {
		fmt.Fprintln(os.Stderr, "用法: NeteaseBedrockGateway -u 用户名 -p 密码 -target 服务器IP/域名:端口 [-server-address 房主地址] [-room-name 名称] [-capacity 容量] [-room-password 密码] [-map-id ID] [-protocol-id ID] [-level-id 版本] [-game-type 类型] [-version-string 版本字符串] [-room-file 落盘文件] [-keepalive 间隔]")
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
	1: "Login",
	2: "PlayStatus",
	3: "ServerToClientHandshake",
	4: "ClientToServerHandshake",
	5: "Disconnect",
	6: "ResourcePacksInfo",
	7: "ResourcePackStack",
	8: "ResourcePackClientResponse",
	9: "Text",
	10: "SetTime",
	11: "StartGame",
	12: "AddPlayer",
	13: "AddActor",
	14: "RemoveActor",
	15: "AddItemActor",
	17: "TakeItemActor",
	18: "MoveActorAbsolute",
	19: "MovePlayer",
	20: "PassengerJump",
	21: "UpdateBlock",
	22: "AddPainting",
	23: "TickSync",
	25: "LevelEvent",
	26: "BlockEvent",
	27: "ActorEvent",
	28: "MobEffect",
	29: "UpdateAttributes",
	30: "InventoryTransaction",
	31: "MobEquipment",
	32: "MobArmourEquipment",
	33: "Interact",
	34: "BlockPickRequest",
	35: "ActorPickRequest",
	36: "PlayerAction",
	38: "HurtArmour",
	39: "SetActorData",
	40: "SetActorMotion",
	41: "SetActorLink",
	42: "SetHealth",
	43: "SetSpawnPosition",
	44: "Animate",
	45: "Respawn",
	46: "ContainerOpen",
	47: "ContainerClose",
	48: "PlayerHotBar",
	49: "InventoryContent",
	50: "InventorySlot",
	51: "ContainerSetData",
	52: "CraftingData",
	54: "GUIDataPickItem",
	55: "AdventureSettings",
	56: "BlockActorData",
	57: "PlayerInput",
	58: "LevelChunk",
	59: "SetCommandsEnabled",
	60: "SetDifficulty",
	61: "ChangeDimension",
	62: "SetPlayerGameType",
	63: "PlayerList",
	64: "SimpleEvent",
	65: "Event",
	66: "SpawnExperienceOrb",
	67: "ClientBoundMapItemData",
	68: "MapInfoRequest",
	69: "RequestChunkRadius",
	70: "ChunkRadiusUpdated",
	72: "GameRulesChanged",
	73: "Camera",
	74: "BossEvent",
	75: "ShowCredits",
	76: "AvailableCommands",
	77: "CommandRequest",
	78: "CommandBlockUpdate",
	79: "CommandOutput",
	80: "UpdateTrade",
	81: "UpdateEquip",
	82: "ResourcePackDataInfo",
	83: "ResourcePackChunkData",
	84: "ResourcePackChunkRequest",
	85: "Transfer",
	86: "PlaySound",
	87: "StopSound",
	88: "SetTitle",
	89: "AddBehaviourTree",
	90: "StructureBlockUpdate",
	91: "ShowStoreOffer",
	92: "PurchaseReceipt",
	93: "PlayerSkin",
	94: "SubClientLogin",
	95: "AutomationClientConnect",
	96: "SetLastHurtBy",
	97: "BookEdit",
	98: "NPCRequest",
	99: "PhotoTransfer",
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

func (debugLogger) Trace(msg string)          {}
func (debugLogger) Tracef(string, ...any)     {}
func (debugLogger) Debug(msg string)          { if debugLog { log.Printf("[pion] %s", msg) } }
func (debugLogger) Debugf(f string, a ...any) { if debugLog { log.Printf("[pion] "+f, a...) } }
func (debugLogger) Info(msg string)           { if debugLog { log.Printf("[pion] %s", msg) } }
func (debugLogger) Infof(f string, a ...any)  { if debugLog { log.Printf("[pion] "+f, a...) } }
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
