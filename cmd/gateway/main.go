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
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net"
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

type session struct {
	raknetConn *raknet.Conn
	enc        *packet.Encoder
	dec        *packet.Decoder
}

func main() {
	var (
		username     = flag.String("u", "", "4399 用户名")
		password     = flag.String("p", "", "4399 密码")
		roomName     = flag.String("room-name", "NeteaseBedrockGateway Host Room", "房间名称")
		roomCapacity = flag.Uint("capacity", 8, "房间容量")
		roomPassword = flag.String("room-password", "", "房间密码（可留空）")
		target       = flag.String("target", "", "目标服务器地址（Geyser/BDS 的 RakNet 端口，必填，例如 服务器IP/域名:端口）")
		mapID        = flag.Uint64("map-id", 0, "房间 MapID（游戏版本标识）")
		protocolID   = flag.Uint("protocol-id", 42, "房间 ProtocolID（默认 42 匹配真实房间）")
		levelID      = flag.String("level-id", "", "房间 LevelID（版本标识字符串）")
		gameType     = flag.Uint("game-type", 0, "房间 GameType")
		versionStr   = flag.String("version-string", "1.21.120.0", "房间游戏版本字符串（玩家校验用）")
		roomFile     = flag.String("room-file", "room.json", "房间信息落盘文件（同时写同名 .txt 只存房间号；空字符串=不落盘）")
		keepalive    = flag.Duration("keepalive", 25*time.Second, "房间存活检查间隔（0=关闭；连续 3 次查不到即自动重建房间）")
	)
	flag.Parse()
	if *username == "" || *password == "" || *target == "" {
		fmt.Fprintln(os.Stderr, "用法: NeteaseBedrockGateway -u 用户名 -p 密码 -target 服务器IP/域名:端口 [-room-name 名称] [-capacity 容量] [-room-password 密码] [-map-id ID] [-protocol-id ID] [-level-id 版本] [-game-type 类型] [-version-string 版本字符串] [-room-file 落盘文件] [-keepalive 间隔]")
		if *username != "" && *password != "" && *target == "" {
			fmt.Fprintln(os.Stderr, "错误: -target 必填（玩家流量转发目标，例如 -target 服务器IP/域名:49780）")
		}
		flag.Usage()
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
	log.Printf("[房主] 目标服务器: %s，房间信息落盘: %s，存活检查间隔: %s", *target, *roomFile, *keepalive)
	g := &gateway{cfg: gatewayConfig{
		username:      *username,
		password:      *password,
		roomName:      *roomName,
		roomCapacity:  *roomCapacity,
		roomPassword:  *roomPassword,
		target:        *target,
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
			// 未知包 ID 不应导致断开：记录并继续（可能是未实现的包类型）
			if strings.Contains(err.Error(), "未知包 ID") {
				log.Printf("[房主] %v（继续监听）", err)
				continue
			}
			log.Printf("[房主] 中转连接关闭: %v", err)
			return
		}
		switch p := pk.(type) {
		case *packet.TanNewGuestResponse:
			log.Printf("[房主] ★ 新玩家加入房间: ErrorCode=%d 玩家数=%d", p.ErrorCode, len(p.PlayerIDList))
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
//	目标服务器（be.4f4t.top:49780，RakNet + Minecraft）
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
				return
			}
			relayLogLine("[玩家→(不可靠)]", data)
		}
	}()

	// 连接目标服务器（go-raknet 兼容 Geyser/BDS 的 Secure cookie 握手）。
	// 带超时 + 重试：go-raknet 在无 deadline 的 context 下会无限重试。
	var netConn net.Conn
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

	done := make(chan struct{}, 2)

	// 玩家 → 目标服务器
	go func() {
		defer func() { done <- struct{}{} }()
		// 先转发 dial 期间缓冲的包
		for data := range pending {
			log.Printf("[转发] 玩家→服务器 %d 字节: %x", len(data), data[:min(len(data), 64)])
			relayLogLine("[玩家→服务器]", data)
			out := playerToServer(&rstate, data)
			if _, err := netConn.Write(out); err != nil {
				return
			}
		}
	}()

	// 目标服务器 → 玩家
	go func() {
		defer func() { done <- struct{}{} }()
		buf := make([]byte, 65536)
		for {
			n, err := netConn.Read(buf)
			if err != nil {
				return
			}
			log.Printf("[转发] 服务器→玩家 %d 字节: %x", n, buf[:min(n, 64)])
			relayLogLine("[服务器→玩家]", buf[:n])
			out, err := serverToPlayer(&rstate, buf[:n])
			if err != nil {
				log.Printf("[转发] 服务器→玩家转译失败: %v", err)
				return
			}
			if _, err := playerConn.Write(out); err != nil {
				return
			}
		}
	}()

	<-done
	log.Printf("[转发] 玩家连接结束")
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

// raknetDial 用 sandertv/go-raknet 连接目标服务器（兼容 BDS/Geyser/cloudburst）。
// 强制 10 秒超时，避免 go-raknet 无限重试。
func raknetDial(ctx context.Context, target string) (net.Conn, error) {
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

func readPacket(decoder *packet.Decoder) (packet.Packet, error) {
	pkData, err := decoder.Decode()
	if err != nil {
		return nil, err
	}
	buf := bytes.NewBuffer(pkData)
	reader := encoding.NewReader(buf)
	header := packet.Header{}
	if err := header.Read(buf); err != nil {
		return nil, err
	}
	pk := packet.NewServerPool()[header.PacketID]
	if pk == nil {
		return nil, fmt.Errorf("未知包 ID: %d (数据=%x)", header.PacketID, pkData)
	}
	pk.Marshal(reader)
	return pk, nil
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

// debugLogger 输出 pion 各子系统（ICE/DTLS/SCTP）的全部日志到 stderr。
type debugLogger struct{}

func (debugLogger) Trace(msg string)          {}
func (debugLogger) Tracef(string, ...any)     {}
func (debugLogger) Debug(msg string)          { log.Printf("[pion] %s", msg) }
func (debugLogger) Debugf(f string, a ...any) { log.Printf("[pion] "+f, a...) }
func (debugLogger) Info(msg string)           { log.Printf("[pion] %s", msg) }
func (debugLogger) Infof(f string, a ...any)  { log.Printf("[pion] "+f, a...) }
func (debugLogger) Warn(msg string)           { log.Printf("[pion] %s", msg) }
func (debugLogger) Warnf(f string, a ...any)  { log.Printf("[pion] "+f, a...) }
func (debugLogger) Error(msg string)          { log.Printf("[pion] %s", msg) }
func (debugLogger) Errorf(f string, a ...any) { log.Printf("[pion] "+f, a...) }

type debugLoggerFactory struct{}

func (debugLoggerFactory) NewLogger(string) logging.LeveledLogger { return debugLogger{} }
