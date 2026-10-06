# NeteaseBedrockGateway

**用程序开一个网易「本地联机」房间，让网易版 Minecraft（中国版）玩家用原版客户端凭房间号，直接进你的 Geyser / 基岩版服务器。**

> 玩家零安装：**不需要 mod、不需要改包、不需要客户端插件**，只要在网易客户端「本地联机」里输入房间号。
>
> **English** — A Go gateway that programmatically creates a NetEase (China) Minecraft "LAN" room via 4399 login + TanLobby, then relays every player's Bedrock traffic to your Geyser / BDS server. Players use the stock NetEase client and just type the room number. No client-side modifications required.

<p>
<img alt="Go" src="https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go">
<img alt="Platform" src="https://img.shields.io/badge/platform-Windows%20%7C%20Linux%20%7C%20macOS-lightgrey">
<img alt="License" src="https://img.shields.io/badge/license-AGPL--3.0-blue">
</p>

---

## 目录

- [特性](#特性)
- [工作原理](#工作原理)
- [编译前置：本仓库不能直接 go build](#编译前置本仓库不能直接-go-build)
- [编译](#编译)
  - [Windows](#windows)
  - [Linux / macOS](#linux--macos)
  - [交叉编译](#交叉编译)
- [使用方法](#使用方法)
  - [1. 启动网关](#1-启动网关)
  - [2. 玩家进服](#2-玩家进服)
  - [3. 目标服务器要求](#3-目标服务器要求)
  - [4. 常驻运行](#4-常驻运行)
- [命令行参数](#命令行参数)
- [房间状态文件](#房间状态文件)
- [目录结构](#目录结构)
- [诊断工具](#诊断工具)
- [故障排查](#故障排查)
- [常见问题](#常见问题)
- [已知限制](#已知限制)
- [免责声明](#免责声明)
- [依赖与许可（Dependencies & Licenses）](#依赖与许可dependencies--licenses)

---

## 特性

| 特性          | 说明                                                               |
| ----------- | ---------------------------------------------------------------- |
| **程序开房**    | 不需要一台真的开着游戏的手机/模拟器当房主，本程序就是房主                                    |
| **原版客户端进服** | 玩家在网易客户端「本地联机」输入房间号即可，无需任何客户端改动                                  |
| **字节级转发**   | 玩家 NetherNet（WebRTC/SCTP）数据通道上的 Bedrock 数据，原样透传到目标服务器的 RakNet 端口 |
| **房间自动看护**  | 中转连接断开、房间被网易回收时**自动重新开房**；房间号落盘到 `room.json` / `room.txt`        |
| **保活与状态**   | 周期性确认房间仍在，并打印在线人数 / 运行时长 / 重建次数                                  |
| **多玩家**     | 每个玩家一条独立转发链路，互不影响                                                |
|  **自带诊断工具** | 33 个排查/逆向小工具（转发包解码、Java 协议探测、抓包分析……）                             |

---

## 工作原理

```
        网易原版客户端                                      你的服务器
   ┌────────────────────────────────┐          ┌──────────────────────────────┐
   │ 本地联机 → 输入房间号 824615   │          │   Geyser (UDP 端口 )         │
   └───────────────┬────────────────┘          │     └── Velocity → Java 后端 │
                   │ ① 按房间号进房           └───────────────▲──────────────┘
                   ▼                                          │ ④ RakNet
   ┌────────────────────────────────┐                         │   [0xFE][批次]
   │ 网易中转服务器 / 信令服务      │                         │
   └───────────────┬────────────────┘          ┌──────────────┴───────────────┐
                   │ ② NetherNet（WebRTC）      │  本程序 NeteaseBedrockGateway│
                   ▼                            │  ├ 4399 登录 + x19 认证       │
   ┌────────────────────────────────────────────┤  ├ TanLobby 开房            │
   │ ③ 玩家 ↔ 网关：SCTP 数据通道               │  ├ TanNotifyServerReady 上报  │
   │    [分段数][Minecraft 批次]                │  └ 字节级双向透传             │
   └────────────────────────────────────────────┴──────────────────────────────┘
```

1. **4399 登录 + x19 认证** → 拿到 `entity_id` + token。
2. **开房凭据**：向网易取中转服务器地址与 RakNet/信令密钥。
3. **连接中转 + 建房**：`TanLoginRequest`（明文）→ `TanCreateRoomRequest`（chacha8，尾部追加房主的 `NetherNetID` 与 `ServerAddress`）→ 服务器返回 **RoomID（房间号）**。
4. **信令 + NetherNet 监听**：WebSocket 连信令服务器，起 WebRTC 监听等待玩家。
5. **「开始游戏」机制**：玩家进房时服务器向房主发 `TanNewGuestResponse`(ID 4)，房主必须回 **`TanNotifyServerReady`**(ID 7，含 `NetherNetID` + `ServerAddress`)，服务器再广播给玩家 —— 没有单独的「开始游戏」请求。
6. **数据面**：网易侧消息 = `[分段数][Bedrock 批次]`，RakNet 侧 = `[0xFE][Bedrock 批次]`；网关只做 `0xFE` 的增删与字节透传，不解析协议、不参与 Bedrock 加密（网易局域网流程本身不加密）。
7. **看护**：主循环监控「中转连接是否还在」「房间是否还在房间列表里」，任一失效即重建房间并更新落盘文件。

> 逆向细节与报文格式见 [docs/troubleshooting.md](docs/troubleshooting.md)。

---
## 编译

**要求**：Go **1.25+**（`go.mod` 声明 `go 1.25`）、**无需 CGO**（全平台可静态构建）。

> 三个外部依赖已随仓库 `vendor/` 提交，**clone 下来直接编译即可**，不需要额外准备模块（也不需要联网拉依赖）。
### Windows

```powershell
git clone https://github.com/DHY0627/NeteaseBedrockGateway.git
cd NeteaseBedrockGateway

go build -o NeteaseBedrockGateway.exe ./cmd/gateway

# 可选：顺便编译常用诊断工具
go build -o bin/relaydecode.exe ./cmd/diag/relaydecode
go build -o bin/javaprobe.exe   ./cmd/diag/javaprobe

.\NeteaseBedrockGateway.exe -u "4399账号" -p "密码" -target 服务器IP/域名:端口
```

### Linux / macOS

```bash
git clone https://github.com/DHY0627/NeteaseBedrockGateway.git
cd NeteaseBedrockGateway

go build -o NeteaseBedrockGateway ./cmd/gateway
chmod +x NeteaseBedrockGateway

./NeteaseBedrockGateway -u "4399账号" -p "密码" -target 服务器IP/域名:端口
```
### 全部组件一起编译（可选）

```bash
# Linux/macOS
go build -o bin/ ./cmd/...

# Windows PowerShell
Get-ChildItem .\cmd -Directory | ForEach-Object {
  go build -o "bin\$($_.Name).exe" "./cmd/$($_.Name)"
}
```

---

## 使用方法

### 1. 启动网关（名称设置其实没用）

```bash
# Windows
NeteaseBedrockGateway.exe -u "房主4399账号" -p "密码" -room-name "我的服务器" -target 服务器IP/域名:端口

# Linux
./NeteaseBedrockGateway -u "房主4399账号" -p "密码" -room-name "我的服务器" -target 服务器IP/域名:端口
```

启动成功后日志：

```
[房主] 目标服务器: 服务器IP/域名:端口，房间信息落盘: room.json，存活检查间隔: 25s
[1/6] 认证成功: uid=742343904
[2/6] 凭据就绪: raknet=42.186.165.232:10007 signaling=42.186.165.232:8899
[3/6] ★ 房间创建成功 RoomID=824615
[4/6] 房间可查询: HID=2889827552 SRV=8361 RoomUniqueID=3541695895718126
[6/6] 请在网易客户端"本地联机"输入房间号 824615 加入
[房主] NetherNet 监听中（NetworkID=11453984968413808426，房间号=123456），等待玩家加入 ...
[房主] 房间 824615 存活（在线 0 人，累计 0 人，已运行 25s，重建 0 次）
```

**房间号就是 `RoomID`**，也写在 `room.txt` / `room.json` 里，方便脚本读取。

### 2. 玩家进服

1. 玩家打开网易版 Minecraft → **「本地联机」**
2. 选择「输入房间号」→ 填入网关打印的房间号（示例 `824615`）
3. 进房后客户端会自动连接网关并开始加载你的服务器

> 建议加 `-room-password "密码"`，否则任何人知道房间号都能进。

### 3. 目标服务器要求

网易客户端走的是**非标准 Bedrock 链路**（RakNet 协议版本 8、自定义协议版本、不做 Bedrock 层加密），标准 Geyser 无法直接对接，需要在你的 Geyser 上安装配套扩展：

- 扩展仓库：[GeyserNetease](../GeyserNetease)（本项目的配套扩展）
- 安装位置（按平台）：
  - Standalone：`extensions/GeyserNeteaseExtension.jar`
  - Velocity：`plugins/Geyser-Velocity/extensions/GeyserNeteaseExtension.jar`
  - BungeeCord：`plugins/Geyser-BungeeCord/extensions/GeyserNeteaseExtension.jar`
  - Spigot/Paper：`plugins/Geyser-Spigot/extensions/GeyserNeteaseExtension.jar`
- **必须**给扩展设置真实地址（否则握手 hostname 为空，会在 `Geyser → 代理` 这一跳被静默掐断）：

  ```
  -DGeyserNetease.ServerAddress=你的域名:端口      例如 be.4f4t.top:49780
  ```

- `-target` 指向 **Geyser 的 RakNet 端口**（UDP，示例里的 `49780`）。

### 4. 常驻运行

**Linux（systemd）** — `/etc/systemd/system/netease-gateway.service`：

```ini
[Unit]
Description=NeteaseBedrockGateway
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=/opt/netease-gateway
ExecStart=/opt/netease-gateway/NeteaseBedrockGateway -u "4399账号" -p "密码" -room-name "我的服务器" -target be.4f4t.top:49780
Restart=always
RestartSec=10
# 房间号写进 /opt/netease-gateway/room.txt

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now netease-gateway
journalctl -u netease-gateway -f          # 看日志
cat /opt/netease-gateway/room.txt         # 看当前房间号
```

**Windows**：用 `nssm` / 任务计划程序把它做成开机自启服务，工作目录设为 exe 所在目录。

> 密码经命令行传入，注意 shell 历史与 systemd unit 文件的权限（`chmod 600`）。

---

## 命令行参数

| 参数                | 默认值                               | 说明                                       |
| ----------------- | --------------------------------- | ---------------------------------------- |
| `-u`              | —                                 | 4399 用户名（**房主账号**，必填）                    |
| `-p`              | —                                 | 4399 密码（必填）                              |
| `-room-name`      | `NeteaseBedrockGateway Host Room` | 房间名称（其实没用）                               |
| `-capacity`       | `8`                               | 房间容量                                     |
| `-room-password`  | 空                                 | 房间密码（留空 = 无密码）                           |
| `-target`         | 空（**必填**）                         | 转发目标：**Geyser / BDS 的 RakNet 端口**，例如 `服务器IP/域名:49780` |
| `-map-id`         | `0`                               | 房间 MapID（游戏版本标识）                         |
| `-protocol-id`    | `42`                              | 房间 ProtocolID（默认 42 与原版房间一致）             |
| `-level-id`       | 空                                 | 房间 LevelID（版本标识字符串）                      |
| `-game-type`      | `0`                               | 房间 GameType                              |
| `-version-string` | `1.21.120.0`                      | 房间游戏版本字符串（玩家进房时校验）                       |
| `-room-file`      | `room.json`                       | 房间信息落盘文件；同时写同名 `.txt`（只存房间号）。空字符串 = 不落盘  |
| `-keepalive`      | `25s`                             | 房间存活检查间隔；`0` = 关闭。连续 3 次查不到即判定房间被回收并自动重建 |

---

## 房间状态文件

房间号由**网易服务器分配**（无法指定/固定），网关重启或重建后会拿到新号，因此会把当前状态落盘：

`room.json`

```json
{
  "room_id": 824615,
  "room_name": "debug-test",
  "target": "服务器IP/域名:端口",
  "host_nether_id": "11453984968413808426",
  "status": "alive",
  "created_at": "2026-10-06T22:36:23+08:00",
  "updated_at": "2026-10-06T22:36:23+08:00",
  "recreations": 0
}
```

`room.txt`：一行房间号，方便 `cat room.txt` / `type room.txt`。房间失效时 `status` 变 `dead`、`room_id` 归零，随后自动重建并刷新。

---

## 目录结构

```
NeteaseBedrockGateway/
├── cmd/
│   ├── gateway/              ★ 主程序（房主网关）
│   │   ├── main.go           协议细节（TanCreateRoom、connectTan、hostReadLoop、玩家转发）
│   │   └── gateway.go        生命周期（登录/凭据刷新/开房/保活/自动重建/落盘）
│   ├── funauth4399/          4399 登录 + 认证 + 开房凭据（CLI 演示）
│   └── diag/                 诊断与逆向小工具（33 个，见下节）
├── internal/
│   ├── auth/                 4399 OAuth 登录 + x19 认证
│   ├── room/                 开房凭据生成（TanLobbyCreate）
│   └── wplauncher/           4399X19Login 登录库（复制自 DHY0627/4399X19Login，MIT）
├── tools/frida/              逆向网易客户端用的 frida 脚本
├── docs/
│   ├── troubleshooting.md    两个「静默失败」的完整排查记录 + 排查手法
│   └── captures/             抓包与 hex 证据（含账号 token，**默认不入库**）
├── images/                   开发者名单用的图片
├── vendor/                   已 vendor 的三个外部依赖（clone 后可直接编译）
├── room.json / room.txt      运行时房间状态（不入库）
├── host.log / relay.log      运行时日志与转发包记录（不入库）
├── README.md / LICENSE / .gitignore / .gitattributes
└── go.mod / go.sum
```

---

## 诊断工具

位于 `cmd/diag/`，全部是独立 `main`：

```bash
go run ./cmd/diag/<名字>
# 或编译：go build -o bin/<名字> ./cmd/diag/<名字>
```

| 工具 | 用途 |
|---|---|
| `relaydecode` | **把 `relay.log` 的原始 hex 解码成 Bedrock 包列表**（剥 `0xFE`、解压、逐帧解析） |
| `javaprobe` | 直接对 Java 服务端做状态查询/离线登录，独立验证 `Geyser → Velocity` 这一跳 |
| `ghost` | 幽灵玩家：用凭据加入别人开的真实房间，观察真实房主行为 |
| `join` | 用第二个账号走完「查询房间 → 进房 → 连房主」链路 |
| `logindump` / `verifyrebuild` | 解析玩家 Login 包；校验「解析→重建」是否与原始字节一致 |
| `chaininfo` / `x5ucheck` / `jwtdump` / `jwtsig` / `hsdecode` | 身份链与握手 JWT 的结构 / 签名 / 公钥分析 |
| `forge` / `rewrite` | 伪造或改写身份链（探测服务端校验策略） |
| `fecheck` / `deflateprobe` / `dectest` | 帧头（`0xFE`）、deflate、压缩方式实验 |
| `rakdec` / `rakdial` / `rakdbg` / `rakreq2` / `rawrelay` / `gordial` / `udpping` | RakNet 层解析、连接与握手实验 |
| `pcapsum` | 简易 pcap 概览（UDP 流 / 包数 / 时间戳） |
| `tandec` / `tansolve` / `lensim` / `dec412` | TanLobby 报文解码与长度对照 |
| `mctest` | 用 go-raknet + solver 的 minecraft 层连 Geyser 验证完整握手 |

---

## 故障排查

```bash
# 1. 看转发记录（完整 hex：谁发了什么）
cat relay.log            # Windows: type relay.log

# 2. 解码成人可读的包列表
go run ./cmd/diag/relaydecode relay.log

# 3. 单独验证 Java 侧（不经过网易客户端）
go run ./cmd/diag/javaprobe -addr be.4f4t.top:25565 -mode status
go run ./cmd/diag/javaprobe -addr be.4f4t.top:25565 -mode login -name TestPlayer
```

| 症状 | 先看哪里 | 多半是 |
|---|---|---|
| 客户端一直「等待房主开始游戏」 | 网关日志有没有 `新玩家加入房间` + `已向玩家上报 NetherNetID` | `TanNotifyServerReady` 没发或发早了 |
| 客户端连上但**零数据**、90 秒超时、`relay.log` 不生成 | `host.log` 中 `收到玩家连接` 之后 | 依赖 `nemc-tan-lobby-solver` 没打补丁（见[编译前置](#编译前置本仓库不能直接-go-build)） |
| 客户端显示 **`数据流终止`**，Geyser 日志同款，Velocity 无日志 | 扩展嗅探日志（`-DGeyserNetease.Sniff=true`） | Geyser 的 java 握手 hostname 为空 → 设置 `-DGeyserNetease.ServerAddress` |
| Geyser 报「服务器已过期/版本不支持」 | Geyser 日志 | 目标服缺 GeyserNetease 扩展，或扩展版本过旧 |
| 房间突然消失 | 网关日志有没有「房间存活检查失败」 | 房间被网易回收 → 新版会自动重建 |
| 启动就报 4399 登录失败 | —— | 账号密码错误，或触发登录限频（等 1~3 分钟） |

更完整的排查方法论见 [docs/troubleshooting.md](docs/troubleshooting.md)。

---

## 常见问题

**Q：玩家需要装 mod 吗？**
A：不需要。玩家用网易版原版客户端，只需要一个房间号。

**Q：可以固定房间号吗？**
A：不行。房间号由网易服务器分配，网关重建后会变；请用 `room.txt` / `room.json` 读取当前房间号。

**Q：国际版（非网易）基岩版玩家能进吗？**
A：能，通过你服务器上原有的 Geyser 正常进（扩展默认 `only-netease-clients: false`，两者并存）。

**Q：Java 版玩家受影响吗？**
A：完全不受影响。

**Q：一个网关能同时让几个玩家进？**
A：每个玩家一条独立转发链路，理论上受房间容量（`-capacity`）与带宽限制；实测单玩家延迟与直连 Geyser 相当。

**Q：能同时开多个房间吗？**
A：可以，用不同 4399 账号、不同工作目录各跑一个进程即可。

**Q：为什么必须给 Geyser 装扩展？**
A：网易客户端使用 RakNet 协议版本 8、自定义协议版本号，且不做 Bedrock 层加密；标准 Geyser 无法直接对接。

---

## 已知限制

- **房间号不可指定**：由网易服务器分配，每次开房/重建都可能不同（已落盘便于读取）。
- **4399 登录限频**：实测两次登录需间隔 60~180 秒；网关只在凭据连续失败后才重新登录，并对失败做 60 秒退避。
- **同一账号不能既当房主又用客户端进房**：房主账号被网易视为「已在房间中」。
- **目标服务器需要配套扩展**：见[目标服务器要求](#3-目标服务器要求)。
- **协议为逆向所得**：网易更新协议后可能需要跟进；本项目已验证协议版本 **630 / 686 / 766 / 819 / 860**。
- **密码经命令行传入**：注意 shell 历史与 unit 文件权限。

---

## 免责声明

本项目仅用于**协议学习与自用**。使用即意味着：你的 4399/网易账号会执行「开房」行为，存在被风控或封禁的风险，请自行评估并承担后果。请勿用于商业用途、批量开房、绕过付费/风控或任何破坏性场景。

`docs/captures/` 中的抓包证据**包含真实账号的身份链与 token**，已通过 `.gitignore` 排除 —— 请勿提交或外传。

---

## 依赖与许可（Dependencies & Licenses）

### 运行时依赖（三个外部模块）

| 模块                                              | 用途                                       |
| ----------------------------------------------- | ---------------------------------------- |
| `github.com/Happy2018new/nemc-tan-lobby-solver` | 网易 TanLobby / NetherNet（WebRTC/SCTP）协议实现 |
| `github.com/sandertv/go-raknet`（fork）           | RakNet 客户端（连 Geyser / BDS）               |
| `github.com/Yeah114/g79client`                  | 4399 登录 / 房间 API 客户端                     |

> 这三个依赖已随仓库 **`vendor/`** 一起提交（`go mod vendor` 的结果），所以 `git clone` 之后**无需另行准备**，直接 `go build` 即可编译。
> `vendor/` 内代码版权与许可证归各自作者所有（`go mod vendor` 只做复制，不改变许可）。

### 间接依赖（由 `go.sum` / `go mod vendor` 自动带入）

| 分类          | 模块                                                                                                                                                                                                                    | 许可证                           |
| ----------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------- |
| WebRTC / 网络 | `pion/dtls/v3`、`pion/ice/v4`、`pion/stun/v3`、`pion/turn/v4`、`pion/mdns/v2`、`pion/srtp/v3`、`pion/rtp`、`pion/rtcp`、`pion/sdp/v3`、`pion/transport/v3`、`pion/interceptor`、`pion/randutil`、`pion/logging`、`coder/websocket` | MIT                           |
| 加密 / 压缩     | `database64128/chacha8-go`、`go-jose/go-jose/v3`、`klauspost/compress`、`golang/snappy`                                                                                                                                  | 各自仓库为准（多为 MIT/BSD/Apache-2.0） |
| 工具库         | `google/uuid`、`muhammadmuzzammil1998/jsonc`、`ugorji/go/codec`、`df-mc/atomic`、`go-gl/mathgl`、`wlynxg/anet`                                                                                                             | 各自仓库为准                        |
| 标准库扩展       | `golang.org/x/crypto`、`golang.org/x/net`、`golang.org/x/sys`、`golang.org/x/text`                                                                                                                                       | BSD-3-Clause                  |

> 完整列表（含版本与哈希）：`go.mod`、`go.sum`；执行 `go mod vendor` 后还会生成 `vendor/modules.txt`。

### 内嵌的第三方代码

| 位置 | 来源 | 许可证 |
|---|---|---|
| `internal/wplauncher/` | [DHY0627/4399X19Login](https://github.com/DHY0627/4399X19Login) | MIT（原文保留在 `internal/wplauncher/LICENSE`） |
| `tools/frida/` | 本项目自写的逆向脚本 | 同本项目 |

### 思路与实现参考

- [Koud-Wind/Netease-minecraft-LAN-connects-to-Server](https://github.com/Koud-Wind/Netease-minecraft-LAN-connects-to-Server)：Java 版「房主开房 + 引流到服务器」同思路
- [GeyserMC/Geyser](https://github.com/GeyserMC/Geyser)、配套扩展 [GeyserNetease](../GeyserNetease)
- ProtoHax（`dev.sora.relay`）：中继 / 中间层实现参考

### 许可证说明

1. **本项目以 [AGPL-3.0](LICENSE) 发布。**
2. `internal/wplauncher/`（来自 DHY0627/4399X19Login）是 **MIT**，与 AGPL-3.0 **兼容**：它保留自己的许可证原文，本项目的 AGPL 覆盖其余部分。
3. 仅本机自用不涉及分发条款，但仍建议知悉以上内容。以上为工程视角的提醒，**不构成法律意见**。


### 本项目许可摘要

- 主许可证：**GNU Affero General Public License v3.0**（[LICENSE](LICENSE)）
- 内嵌第三方：`internal/wplauncher/` 保留其原始 **MIT** 许可证
- 由于是 AGPL：**如果你把它作为网络服务提供给别人使用，需要按 AGPL 第 13 条向使用者提供完整源码**
# 开发者名单
## 本项目(不包括依赖)都是由以下一位开发者开发，所有代码都是他写的
![Deepseek Hardness|402](./images/coder.png)