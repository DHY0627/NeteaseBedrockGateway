// 房主网关的"看护"逻辑：4399 登录 → 开房 → 信令/NetherNet 监听 → 转发，
// 并在房间失效（中转连接断开、房间被服务器回收）时自动重新开房。
//
// 与 main.go 的分工：
//   - main.go      ：协议细节（TanCreateRoom、connectTan、hostReadLoop、玩家转发、日志）
//   - gateway.go   ：生命周期（登录/凭据刷新/建房间/保活/重建/持久化）
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	funauth "NeteaseBedrockGateway/internal/auth"
	"NeteaseBedrockGateway/internal/room"

	"github.com/Happy2018new/nemc-tan-lobby-solver/core/nethernet"
	"github.com/Happy2018new/nemc-tan-lobby-solver/core/webrtc"
	"github.com/Happy2018new/nemc-tan-lobby-solver/protocol/login/signaling"
	"github.com/Yeah114/g79client"
	"github.com/pion/dtls/v3"
)

// gatewayConfig 是网关的运行参数。
type gatewayConfig struct {
	username, password     string
	roomName, roomPassword string
	roomCapacity           uint
	target                 string // 玩家流量转发目标（Geyser/BDS 的 RakNet 端口）
	serverAddr             string // 上报给网易的房主地址（玩家据此连房主）；为空时回退到 target
	mapID                  uint64
	protocolID             uint8
	levelID                string
	gameType               uint8
	versionString          string

	roomFile  string        // 房间信息落盘文件（默认 room.json，同目录再写一个 room.txt）
	keepalive time.Duration // 房间存活检查间隔，0 表示关闭检查
}

// gateway 保存跨房间重建需要保持的状态。
type gateway struct {
	cfg gatewayConfig

	mu          sync.Mutex
	cli         *g79client.Client  // 4399 客户端（凭据刷新/查询房间用）
	cred        *room.CreateResult // 开房凭据（连接中转用）
	credFails   int                // 连续获取凭据失败次数，达到阈值后强制重新登录
	roomID      uint32
	roomSince   time.Time
	recreations int
	netherID    uint64

	onlinePlayers int32 // 当前在线玩家数
	servedPlayers int64 // 累计服务玩家数
}

// roomRecord 是写入 room.json 的内容，便于外部脚本（例如给玩家播报房间号）读取。
type roomRecord struct {
	RoomID       uint32 `json:"room_id"`
	RoomName     string `json:"room_name"`
	Target       string `json:"target"`
	HostNetherID string `json:"host_nether_id"`
	Status       string `json:"status"` // alive / dead
	CreatedAt    string `json:"created_at,omitempty"`
	UpdatedAt    string `json:"updated_at"`
	Recreations  int    `json:"recreations"`
}

// run 是网关主循环：建立房间并服务玩家；房间失效时自动重建。
func (g *gateway) run(ctx context.Context) error {
	if err := g.login(ctx); err != nil {
		return fmt.Errorf("4399 登录失败: %w", err)
	}

	netherID, err := randUint64()
	if err != nil {
		return fmt.Errorf("生成 NetherNetID 失败: %w", err)
	}
	g.netherID = netherID

	for ctx.Err() == nil {
		host, err := g.ensureRoom(ctx)
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			log.Printf("[房主] 开房失败: %v（15 秒后重试）", err)
			g.sleep(ctx, 15*time.Second)
			continue
		}

		// 每个房间一个子 context：房间失效即取消，所有相关 goroutine 随之退出。
		roomCtx, cancelRoom := context.WithCancel(ctx)
		reason := make(chan string, 1)
		fail := func(r string) {
			select {
			case reason <- r:
			default:
			}
			cancelRoom()
		}

		// 中转连接读循环：它一结束就说明房主与服务器的连接断了 → 需要重建房间。
		go func() {
			hostReadLoop(roomCtx, host, g.netherID, g.netherAddr(), g)
			fail("与中转发服务器的连接已断开")
		}()

		// 保活：周期性确认房间仍在服务器的房间列表里，顺带打印状态行。
		if g.cfg.keepalive > 0 {
			go g.keepaliveLoop(roomCtx, fail)
		}

		g.serve(roomCtx, host)

		cancelRoom()
		_ = host.raknetConn.Close()
		select {
		case r := <-reason:
			g.markRoomDead(r)
		default:
			g.markRoomDead("房间会话结束")
		}
		if ctx.Err() == nil {
			log.Printf("[房主] 5 秒后重新开房 ...")
			g.sleep(ctx, 5*time.Second)
		}
	}

	log.Println("房主网关已停止。")
	return nil
}

// login 做一次 4399 登录 + x19 认证。
func (g *gateway) login(ctx context.Context) error {
	log.Println("[1/6] 4399 登录 + x19 认证 ...")
	cli, _, err := funauth.Login(ctx, g.cfg.username, g.cfg.password, funauth.LoginOptions{})
	if err != nil {
		return err
	}
	g.mu.Lock()
	g.cli = cli
	g.credFails = 0
	g.mu.Unlock()
	log.Printf("[1/6] 认证成功: uid=%s", cli.UserID)
	return nil
}

// refreshCredential 获取（必要时先重新登录）一份可用的开房凭据。
// 4399 登录有频率限制（实测 60~180 秒），所以只在凭据连续失败后才重新登录。
func (g *gateway) refreshCredential(ctx context.Context) error {
	for ctx.Err() == nil {
		g.mu.Lock()
		cli := g.cli
		g.mu.Unlock()

		if cli == nil {
			if err := g.login(ctx); err != nil {
				log.Printf("[房主] 重新登录 4399 失败: %v（60 秒后重试，登录有限频）", err)
				g.sleep(ctx, 60*time.Second)
				continue
			}
			g.mu.Lock()
			cli = g.cli
			g.mu.Unlock()
		}

		cred, err := room.Create(ctx, cli)
		if err != nil {
			g.mu.Lock()
			g.credFails++
			fails := g.credFails
			if fails >= 3 {
				g.cli = nil // 连续失败：判定登录态失效，下一轮重新登录
			}
			g.mu.Unlock()
			log.Printf("[房主] 获取开房凭据失败（第 %d 次）: %v", fails, err)
			g.sleep(ctx, 10*time.Second)
			continue
		}

		g.mu.Lock()
		g.cred = cred
		g.credFails = 0
		g.mu.Unlock()
		log.Printf("[2/6] 凭据就绪: raknet=%s signaling=%s", cred.RaknetServerAddress, cred.SignalingServerAddress)
		return nil
	}
	return ctx.Err()
}

// ensureRoom 建立房间：连接中转 → 创建房间 → 查询确认 → 落盘。
func (g *gateway) ensureRoom(ctx context.Context) (*session, error) {
	attempt := 0
	for ctx.Err() == nil {
		attempt++

		g.mu.Lock()
		cred := g.cred
		cli := g.cli
		g.mu.Unlock()
		if cred == nil || cli == nil {
			if err := g.refreshCredential(ctx); err != nil {
				return nil, err
			}
			g.mu.Lock()
			cred = g.cred
			cli = g.cli
			g.mu.Unlock()
		}

		log.Printf("[3/6] 连接中转服务器并创建房间 ...（第 %d 次尝试）", attempt)
		host, err := connectTan(ctx, cred, cred.UserUniqueID, cred.UserPlayerName)
		if err != nil {
			log.Printf("[房主] 连接中转失败: %v", err)
			g.invalidateCredential()
			continue
		}

		roomID, err := createRoom(ctx, host, g.cfg.roomName, g.cfg.roomCapacity, g.cfg.roomPassword,
			g.cfg.mapID, g.cfg.protocolID, g.cfg.levelID, g.cfg.gameType, g.cfg.versionString,
			g.netherID, g.netherAddr())
		if err != nil {
			_ = host.raknetConn.Close()
			log.Printf("[房主] 创建房间失败: %v", err)
			g.invalidateCredential()
			continue
		}
		log.Printf("[3/6] ★ 房间创建成功 RoomID=%d", roomID)
		g.setRoomAlive(roomID)

		// 4. 查询确认房间已发布
		log.Println("[4/6] 查询房间确认 ...")
		if info, qerr := cli.GetTransferRoomWithName(fmt.Sprintf("%d", roomID)); qerr != nil {
			log.Printf("[4/6] 查询房间失败（可能尚未发布）: %v", qerr)
		} else if info != nil && len(info.List) > 0 {
			log.Printf("[4/6] 房间可查询: HID=%s SRV=%s RoomUniqueID=%s",
				info.List[0].HID, info.List[0].SRV, info.List[0].RoomUniqueID)
		} else {
			log.Println("[4/6] 房间暂不可查询（可能尚未发布）")
		}

		log.Printf("[6/6] 房主网关运行中。房间号=%d，玩家流量将转发到 %s", roomID, g.cfg.target)
		log.Printf("[6/6] 请在网易客户端\"本地联机\"输入房间号 %d 加入", roomID)
		return host, nil
	}
	return nil, ctx.Err()
}

// serve 运行信令 + NetherNet 监听，接受玩家；信令断开自动重连，直到 roomCtx 结束。
func (g *gateway) serve(roomCtx context.Context, host *session) {
	for roomCtx.Err() == nil {
		g.mu.Lock()
		cred := g.cred
		g.mu.Unlock()
		if cred == nil {
			log.Println("[房主] 凭据丢失，准备重新开房")
			return
		}

		wsConn, err := signaling.Dialer{NetworkID: g.netherID}.DialContext(
			roomCtx,
			cred.SignalingServerAddress,
			g.netherID,
			cred.UserUniqueID,
			cred.SignalingSeed,
			cred.SignalingTicket,
		)
		if err != nil {
			log.Printf("[房主] 信令连接失败: %v（5 秒后重试）", err)
			g.sleep(roomCtx, 5*time.Second)
			continue
		}

		// 网易客户端使用 DTLS 1.2 extended master secret + SCTP zero-checksum，
		// 需在 WebRTC API 中开启（否则 SCTP 在 DTLS 上无法完成握手）。
		se := webrtc.SettingEngine{}
		se.SetDTLSExtendedMasterSecret(dtls.RequireExtendedMasterSecret)
		se.EnableSCTPZeroChecksum(true)
		se.LoggerFactory = debugLoggerFactory{}
		api := webrtc.NewAPI(webrtc.WithSettingEngine(se))
		var nl nethernet.ListenConfig
		nl.API = api
		// 子模块的 Info 级日志是逐包 hex，默认只放行 Warn 及以上（-d/--debug 时全放行）
		nl.Log = netherLogger()

		listener, err := nl.Listen(wsConn)
		if err != nil {
			_ = wsConn.Close()
			log.Printf("[房主] nethernet 监听失败: %v（5 秒后重试）", err)
			g.sleep(roomCtx, 5*time.Second)
			continue
		}

		log.Printf("[房主] NetherNet 监听中（NetworkID=%d，房间号=%d），等待玩家加入 ...", g.netherID, g.roomIDValue())

		// roomCtx 结束时关闭 listener，让 Accept 立即返回
		done := make(chan struct{})
		go func() {
			select {
			case <-roomCtx.Done():
				_ = listener.Close()
			case <-done:
			}
		}()

		acceptPlayers(roomCtx, listener, g.cfg.target, g)
		close(done)
		_ = listener.Close()
		_ = wsConn.Close()

		if roomCtx.Err() != nil {
			return
		}
		log.Println("[房主] 信令连接已断开，5 秒后重连 ...")
		g.sleep(roomCtx, 5*time.Second)
	}
}

// keepaliveLoop 周期性确认房间仍存在；连续多次查不到就判定房间被回收，触发重建。
func (g *gateway) keepaliveLoop(roomCtx context.Context, fail func(string)) {
	const maxMisses = 3
	misses := 0
	for {
		g.sleep(roomCtx, g.cfg.keepalive)
		if roomCtx.Err() != nil {
			return
		}

		g.mu.Lock()
		cli := g.cli
		roomID := g.roomID
		g.mu.Unlock()
		if cli == nil || roomID == 0 {
			return
		}

		info, err := cli.GetTransferRoomWithName(fmt.Sprintf("%d", roomID))
		if err == nil && info != nil && len(info.List) > 0 {
			misses = 0
			log.Printf("[房主] 房间 %d 存活（在线 %d 人，累计 %d 人，已运行 %s，重建 %d 次）",
				roomID, atomic.LoadInt32(&g.onlinePlayers), atomic.LoadInt64(&g.servedPlayers),
				time.Since(g.roomSinceValue()).Round(time.Second), g.recreationsValue())
			continue
		}

		misses++
		log.Printf("[房主] 房间存活检查失败（%d/%d）: %v", misses, maxMisses, err)
		if misses >= maxMisses {
			fail(fmt.Sprintf("房间 %d 已被服务器回收", roomID))
			return
		}
	}
}

// ==================== 状态与落盘 ====================

// netherAddr 返回上报给网易的房主地址（网易用 "|" 分隔 host 与 port）。
// 优先用 -server-address；未指定时回退到 -target —— 二者在
// 「Geyser 与网关同机同端口」的部署下本来就相同。
func (g *gateway) netherAddr() string {
	addr := g.cfg.serverAddr
	if addr == "" {
		addr = g.cfg.target
	}
	return strings.ReplaceAll(addr, ":", "|")
}

func (g *gateway) setRoomAlive(roomID uint32) {
	g.mu.Lock()
	g.roomID = roomID
	g.roomSince = time.Now()
	g.mu.Unlock()
	g.persistRoom("alive")
}

func (g *gateway) markRoomDead(reason string) {
	g.mu.Lock()
	g.recreations++
	roomID := g.roomID
	g.roomID = 0
	g.mu.Unlock()
	log.Printf("[房主] 房间 %d 失效（%s），准备重建 ...", roomID, reason)
	g.persistRoom("dead")
}

// invalidateCredential 丢弃当前开房凭据（房间可能因此需要重建）。
func (g *gateway) invalidateCredential() {
	g.mu.Lock()
	g.cred = nil
	g.mu.Unlock()
}

func (g *gateway) roomIDValue() uint32 {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.roomID
}

func (g *gateway) roomSinceValue() time.Time {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.roomSince
}

func (g *gateway) recreationsValue() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.recreations
}

// persistRoom 把当前房间信息写入 room.json / room.txt（外部脚本可读）。
func (g *gateway) persistRoom(status string) {
	if g.cfg.roomFile == "" {
		return
	}
	g.mu.Lock()
	rec := roomRecord{
		RoomID:       g.roomID,
		RoomName:     g.cfg.roomName,
		Target:       g.cfg.target,
		HostNetherID: fmt.Sprintf("%d", g.netherID),
		Status:       status,
		UpdatedAt:    time.Now().Format(time.RFC3339),
		Recreations:  g.recreations,
	}
	if !g.roomSince.IsZero() {
		rec.CreatedAt = g.roomSince.Format(time.RFC3339)
	}
	g.mu.Unlock()

	b, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return
	}
	if err := os.WriteFile(g.cfg.roomFile, append(b, '\n'), 0o644); err != nil {
		log.Printf("[房主] 写房间文件失败: %v", err)
	}

	// 同目录再写一个纯数字的 room.txt，方便脚本/命令直接读
	ext := filepath.Ext(g.cfg.roomFile)
	txtPath := strings.TrimSuffix(g.cfg.roomFile, ext) + ".txt"
	if err := os.WriteFile(txtPath, []byte(fmt.Sprintf("%d\n", rec.RoomID)), 0o644); err != nil {
		log.Printf("[房主] 写房间号文件失败: %v", err)
	}
}

// onRoomPlayers 记录中转上报的房间里玩家数（TanNewGuestResponse）。
func (g *gateway) onRoomPlayers(n int) {
	atomic.StoreInt32(&g.onlinePlayers, int32(n))
}

// onPlayerServed 记录"有一个玩家通过 NetherNet 连上本网关"。
func (g *gateway) onPlayerServed() {
	atomic.AddInt64(&g.servedPlayers, 1)
	atomic.AddInt32(&g.onlinePlayers, 1)
}

// onPlayerLeft 记录玩家连接结束。
func (g *gateway) onPlayerLeft() {
	if n := atomic.AddInt32(&g.onlinePlayers, -1); n < 0 {
		atomic.StoreInt32(&g.onlinePlayers, 0)
	}
}

// sleep 是 ctx 感知的 sleep。
func (g *gateway) sleep(ctx context.Context, d time.Duration) {
	if d <= 0 {
		return
	}
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}
