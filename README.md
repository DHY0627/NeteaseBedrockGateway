# NeteaseBedrockGateway

**用程序开一个网易「本地联机」房间，让网易版 Minecraft（中国版）玩家用原版客户端凭房间号，直接进你的 Geyser / 基岩版服务器。**

> 玩家零安装：**不需要 mod、不需要改包、不需要客户端插件**，只要在网易客户端「本地联机」里输入房间号。
>
> **English** — A Go gateway that programmatically creates a NetEase (China) Minecraft "LAN" room via 4399 login + TanLobby, then relays every player's Bedrock traffic to your Geyser / BDS server. Players use the stock NetEase client and just type the room number. No client-side modifications required.

<p>
<img alt="Go" src="https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go">
<img alt="Platform" src="https://img.shields.io/badge/platform-Windows%20%7C%20Linux%20%7C%20macOS-lightgrey">
<img alt="License" src="https://img.shields.io/badge/license-AGPL--3.0-blue">
<a href="https://github.com/DHY0627/NeteaseBedrockGateway/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/DHY0627/NeteaseBedrockGateway/actions/workflows/ci.yml/badge.svg"></a>
<a href="https://github.com/DHY0627/NeteaseBedrockGateway/releases/latest"><img alt="Release" src="https://img.shields.io/github/v/release/DHY0627/NeteaseBedrockGateway?include_prereleases&label=release"></a>
</p>

---

## 目录

- [特性](#特性)
- [工作原理](#工作原理)
- [下载现成程序（免编译）](#下载现成程序免编译)
- [编译](#编译)
  - [Windows](#windows)
  - [Linux / macOS](#linux--macos)
- [使用方法](#使用方法)
  - [1. 启动网关](#1-启动网关名称设置其实没用)
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
   │ 本地联机 → 输入房间号 123456   │          │   Geyser (UDP 19132)         │
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
## 下载现成程序（免编译）

不想装 Go 的话，直接去 [**Releases**](https://github.com/DHY0627/NeteaseBedrockGateway/releases/latest) 下载对应平台的可执行文件，解压即用：

| 系统 | 架构 | 文件 |
|---|---|---|
| Windows | x64（64 位） | `NeteaseBedrockGateway-windows-amd64.exe` |
| Windows | x86（32 位） | `NeteaseBedrockGateway-windows-386.exe` |
| Windows | ARM64 | `NeteaseBedrockGateway-windows-arm64.exe` |
| Linux | x64（64 位） | `NeteaseBedrockGateway-linux-amd64` |
| Linux | x86（32 位） | `NeteaseBedrockGateway-linux-386` |
| Linux | ARM64 | `NeteaseBedrockGateway-linux-arm64` |
| Linux | ARM 32 位 | `NeteaseBedrockGateway-linux-armv7` |
| macOS | Intel | `NeteaseBedrockGateway-darwin-amd64` |
| macOS | Apple 芯片 | `NeteaseBedrockGateway-darwin-arm64` |
| 诊断工具 | Linux x64 | `NeteaseBedrockGateway-diag-linux-amd64.tar.gz` |

> 分不清架构？Linux 执行 `uname -m`：`x86_64` → amd64、`aarch64` → arm64、`armv7l` → armv7、`i686` → 386。
> Windows 看「设置 → 系统 → 关于 → 系统类型」。选错了会报「不是有效的 Win32 应用程序」或 `cannot execute binary file`。

校验完整性：`sha256sum -c SHA256SUMS.txt`

> 这些文件由 [GitHub Actions](.github/workflows/release.yml) 在打 tag 时自动构建，源码与产物一一对应。

---

## 编译

**要求**：Go **1.25+**（`go.mod` 声明 `go 1.25`）、**无需 CGO**（全平台可静态构建）、**git**。

> ⚠️ **依赖用 git submodule 管理，clone 之后必须先拉取，否则编译不过。**
> 三个外部依赖位于 `third_party/`，分别指向各自的 Git 仓库（两个是本项目的 fork）：
> `third_party/nemc-tan-lobby-solver`、`third_party/go-raknet`、`third_party/g79client`。
> 拉取命令（**clone 后第一件事**）：`git submodule update --init --recursive`，或者 clone 时直接带上 `--recursive`：
> ```bash
> git clone --recursive https://github.com/DHY0627/NeteaseBedrockGateway.git
> ```
> 之后 `go build` 就正常了；`go.mod` 用相对路径 `./third_party/...` 指向它们，**不要**再执行 `go mod vendor`。
> 只有第三方**间接**依赖（pion、x/crypto 等）仍需联网从模块代理下载，`go.sum` 里已固定版本与哈希。

### Windows

```powershell
git clone --recursive https://github.com/DHY0627/NeteaseBedrockGateway.git
cd NeteaseBedrockGateway

go build -o NeteaseBedrockGateway.exe ./cmd/gateway

# 可选：顺便编译常用诊断工具
go build -o bin/relaydecode.exe ./cmd/diag/relaydecode
go build -o bin/javaprobe.exe   ./cmd/diag/javaprobe

.\NeteaseBedrockGateway.exe -u "4399账号" -p "密码" -target 服务器IP/域名:端口
```

### Linux / macOS

```bash
git clone --recursive https://github.com/DHY0627/NeteaseBedrockGateway.git
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
[3/6] ★ 房间创建成功 RoomID=123456
[4/6] 房间可查询: HID=1000000001 SRV=10001 RoomUniqueID=1000000000000001
[6/6] 请在网易客户端"本地联机"输入房间号 123456 加入
[房主] NetherNet 监听中（NetworkID=10000000000000000001，房间号=123456），等待玩家加入 ...
[房主] 房间 123456 存活（在线 0 人，累计 0 人，已运行 25s，重建 0 次）
```

**房间号就是 `RoomID`**，也写在 `room.txt` / `room.json` 里，方便脚本读取。

### 2. 玩家进服

1. 玩家打开网易版 Minecraft → **「本地联机」**
2. 选择「输入房间号」→ 填入网关打印的房间号（示例 `123456`）
3. 进房后客户端会自动连接网关并开始加载你的服务器

> 建议加 `-room-password "密码"`，否则任何人知道房间号都能进。

### 3. 目标服务器要求

网易客户端走的是**非标准 Bedrock 链路**（RakNet 协议版本 8、自定义协议版本、不做 Bedrock 层加密），标准 Geyser 无法直接对接，需要在你的 Geyser 上安装配套扩展：

- 扩展仓库：[DHY0627/GeyserNetease](https://github.com/DHY0627/GeyserNetease)（本项目配套的 fork，基于 [LoHJG/GeyserNetease](https://github.com/LoHJG/GeyserNetease)，适配 Geyser 2.11.3 并修复了下面的 hostname 问题）
- **直接下载预编译 jar**（不用自己编译）：
  [`GeyserNeteaseExtension.jar`](https://github.com/DHY0627/GeyserNetease/raw/main/dist/GeyserNeteaseExtension.jar)

  | 项 | 值 |
  |---|---|
  | 大小 / SHA256 | 3,118,355 字节 / `3388f14b508d6c5e0dea5021ceb27647dded65b7b48a1531c78a3a51216dbdf9` |
  | 适配 Geyser | 2.11.3（扩展版本 1.1.0） |

- 安装位置（按平台，是 Geyser 的 **`extensions/` 子目录**，不是 `plugins/` 根目录）：
  - Standalone：`extensions/GeyserNeteaseExtension.jar`
  - Velocity：`plugins/Geyser-Velocity/extensions/GeyserNeteaseExtension.jar`
  - BungeeCord：`plugins/Geyser-BungeeCord/extensions/GeyserNeteaseExtension.jar`
  - Spigot/Paper：`plugins/Geyser-Spigot/extensions/GeyserNeteaseExtension.jar`
- **必须**给扩展设置真实地址（否则握手 hostname 为空，会在 `Geyser → 代理` 这一跳被静默掐断）：

  ```
  # 加在 java 命令的 -jar 之前，例如：
  java -DGeyserNetease.ServerAddress=你的域名:19132 -jar geyser.jar
  ```

  > jar 里的默认值是脱敏示例 `example.com:19132`，**不能直接用**，必须换成你自己的地址。

- `-target` 指向 **Geyser 的 RakNet 端口**（UDP，Geyser 默认 `19132`）。

> ⚠️ **`-target` 和 `-server-address` 是两个不同的东西，别搞混：**
>
> | 参数 | 含义 | 谁去连它 |
> |---|---|---|
> | `-target` | 玩家流量**转发到哪**（你的 Geyser / BDS） | 网关自己去连 |
> | `-server-address` | 玩家**去哪找房主**（网易 NetherNet 入口） | **玩家**去连 |
>
> `-server-address` 留空时**继承 `-target`**。绝大多数部署里二者就是同一个地址（网关与 Geyser 同机、共用同一个 RakNet 端口），
> 所以通常不用写。但只要它们不同，**必须显式指定 `-server-address`**，否则会把错误的地址广播给玩家，
> 表现为**玩家进房后一直卡在「等待房主开始游戏」**，且网关日志里**看不到 `★ 收到玩家连接`**。
>
> 启动时日志会同时打印这两个值，对着确认一遍：
> ```
> [房主] 转发目标: geyser.example.com:19132，上报房主地址: nether.example.com:49780，...
> ```

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
ExecStart=/opt/netease-gateway/NeteaseBedrockGateway -u "4399账号" -p "密码" -room-name "我的服务器" -target example.com:19132
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
| `-target`         | 空（**必填**）                         | **转发目标**：玩家流量连到哪（Geyser / BDS 的 RakNet 端口），例如 `服务器IP/域名:19132` |
| `-server-address` | 空（= 继承 `-target`）                 | **上报给网易的房主地址**：玩家去哪找房主（NetherNet 入口）。只有与 `-target` 不同才需要写 |
| `-map-id`         | `0`                               | 房间 MapID（游戏版本标识）                         |
| `-protocol-id`    | `42`                              | 房间 ProtocolID（默认 42 与原版房间一致）             |
| `-level-id`       | `r6YhQdny6LU=`                    | 房间**游戏版本标识**（base64 的 8 字节）。⚠️ **别改成空**：留空会导致玩家能进房间但**无法开始游戏**（详见下方说明） |
| `-game-type`      | `0`                               | 房间 GameType                              |
| `-version-string` | `1.21.120.0`                      | 房间游戏版本字符串（玩家进房时校验）                       |
| `-room-file`      | `room.json`                       | 房间信息落盘文件；同时写同名 `.txt`（只存房间号）。空字符串 = 不落盘  |
| `-keepalive`      | `25s`                             | 房间存活检查间隔；`0` = 关闭。连续 3 次查不到即判定房间被回收并自动重建 |

> ⚠️ **`-level-id` 不要留空。**
> 它是房间的「游戏版本标识」（`r6YhQdny6LU=` 解码后是 8 字节 `AF A6 21 41 D9 F2 E8 B5`，对应 1.21.120.0），
> 和 `-version-string` 一起放进 RoomTips，供网易客户端进房时做**版本一致性校验**。
> 留空的话：玩家**能进房间**、网关也能收到 `★ 新玩家加入房间`，但客户端**不会去建立 NetherNet 连接** ——
> 表现为一直卡在「等待房主开始游戏」，网关日志里**永远不会出现 `★ 收到玩家连接`，也没有任何 ICE 日志**。
> 该值来自逆向真实客户端的建房报文（见 `cmd/diag/lensim`）。若玩家用的游戏版本与此不同，可能需要更换。

---

## 房间状态文件

房间号由**网易服务器分配**（无法指定/固定），网关重启或重建后会拿到新号，因此会把当前状态落盘：

`room.json`

```json
{
  "room_id": 123456,
  "room_name": "example-room",
  "target": "example.com:19132",
  "host_nether_id": "10000000000000000001",
  "status": "alive",
  "created_at": "2026-01-01T00:00:00+08:00",
  "updated_at": "2026-01-01T00:00:00+08:00",
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
├── scripts/                  build.ps1 / build.sh 本地交叉编译脚本
├── .github/workflows/        ci.yml（push 校验）+ release.yml（打 tag 自动发版）
├── docs/
│   ├── troubleshooting.md    两个「静默失败」的完整排查记录 + 排查手法
│   └── captures/             抓包与 hex 证据（含账号 token，**默认不入库**）
├── images/                   开发者名单用的图片
├── third_party/              三个外部依赖（git submodule：solver / go-raknet / g79client）
├── .gitmodules               上面三个 submodule 的地址与分支
├── room.json / room.txt      运行时房间状态（不入库）
├── relay.log                 转发包记录（不入库；主日志是标准输出，重定向保存即可，例如 > host.log）
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
go run ./cmd/diag/javaprobe -addr example.com:25565 -mode status
go run ./cmd/diag/javaprobe -addr example.com:25565 -mode login -name TestPlayer
```

| 症状 | 先看哪里 | 多半是 |
|---|---|---|
| 客户端能进房间但**卡在「等待房主开始游戏」**，日志有 `★ 新玩家加入房间` 却**没有** `★ 收到玩家连接`，也**没有任何 ICE 日志** | 启动参数里的 `-level-id` 是不是空的 | **`-level-id` 留空** → 房间版本标识无效，客户端不做 NetherNet 连接（这是最容易踩的坑） |
| 客户端一直「等待房主开始游戏」，但日志里**有** `已向玩家上报 NetherNetID` | `TanNotifyServerReady` 是否发出 | `TanNotifyServerReady` 没发或发早了 |
| 客户端连上但**零数据**、90 秒超时、`relay.log` 不生成 | 启动日志（标准输出）里 `收到玩家连接` 之后 | 依赖 `nemc-tan-lobby-solver` 没打补丁（见 [依赖与许可](#依赖与许可dependencies--licenses) 的 fork 说明） |
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

> 这三个依赖以 **git submodule** 的形式放在 `third_party/` 下（见 `.gitmodules`），
> 所以 `git clone` 之后要执行一次 `git submodule update --init --recursive` 才能编译（或 clone 时直接加 `--recursive`，见上）。
> `go.mod` 用相对路径把它们接进来：`replace <模块路径> => ./third_party/<目录名>`。
> `third_party/` 内代码版权与许可证归各自作者所有（submodule 只是引用，不改变许可）。

其中前两个用的是**我们自己的 fork**（都带了自己的补丁）：

| 依赖 | 本项目的 fork | 上游 | 我们改了什么 |
|---|---|---|---|
| `nemc-tan-lobby-solver` | [DHY0627/nemc-tan-lobby-solver](https://github.com/DHY0627/nemc-tan-lobby-solver) | [UCKETX/nemc-tan-lobby-solver](https://github.com/UCKETX/nemc-tan-lobby-solver)（**无 LICENSE**） | NetherNet 不丢首包、不可靠通道、Geyser Secure Cookie、SCTP CRC32C 小端、TanLobby 编解码补充（共 14 个文件，清单见 fork 的 `FORK-NOTICE.md`） |
| `sandertv/go-raknet` | [DHY0627/go-raknet](https://github.com/DHY0627/go-raknet)（分支 `netease`） | [sandertv/go-raknet](https://github.com/sandertv/go-raknet) `v1.15.1`（MIT） | `conn.go`：`protocolVersion` 由 `11` 改为 `8`（网易客户端只认 RakNet 协议版本 8，发 11 会零数据），清单见 fork 的 `FORK-NOTICE.md` |

> 三个 submodule 就对应上表三个依赖，`third_party/<目录名>` 即各自的检出位置：
> `nemc-tan-lobby-solver` 跟踪 fork 的 `main`（`f2649a1`），`go-raknet` 跟踪 fork 的 `netease` 分支（`2e9d856`，基线 `v1.15.1`），
> `g79client` 直接钉在上游 `UCKETX/g79client` 的 `e837667`（与本项目收到的版本完全一致，没有本地改动）。
> 要升级某个 fork：在 `third_party/<目录名>` 里 `git fetch && git checkout <新提交>`，回到仓库根目录 `git add third_party/<目录名>` 提交这个新指针即可（**不要**再执行 `go mod vendor`）。

> ⚠️ `nemc-tan-lobby-solver` 上游**没有许可证文件**（fork 亦未新增），版权归原作者；本机自用不受分发条款约束，若要再分发请先联系原作者取得许可。

### 间接依赖（不在 `third_party/` 里，由 `go.sum` 自动解析）

| 分类          | 模块                                                                                                                                                                                                                    | 许可证                           |
| ----------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------- |
| WebRTC / 网络 | `pion/dtls/v3`、`pion/ice/v4`、`pion/stun/v3`、`pion/turn/v4`、`pion/mdns/v2`、`pion/srtp/v3`、`pion/rtp`、`pion/rtcp`、`pion/sdp/v3`、`pion/transport/v3`、`pion/interceptor`、`pion/randutil`、`pion/logging`、`coder/websocket` | MIT                           |
| 加密 / 压缩     | `database64128/chacha8-go`、`go-jose/go-jose/v3`、`klauspost/compress`、`golang/snappy`                                                                                                                                  | 各自仓库为准（多为 MIT/BSD/Apache-2.0） |
| 工具库         | `google/uuid`、`muhammadmuzzammil1998/jsonc`、`ugorji/go/codec`、`df-mc/atomic`、`go-gl/mathgl`、`wlynxg/anet`                                                                                                             | 各自仓库为准                        |
| 标准库扩展       | `golang.org/x/crypto`、`golang.org/x/net`、`golang.org/x/sys`、`golang.org/x/text`                                                                                                                                       | BSD-3-Clause                  |

> 完整列表（含版本与哈希）：`go.mod`、`go.sum`。这些是普通模块依赖（不是 submodule），首次编译会自动下载到本机模块缓存。

### 内嵌的第三方代码

| 位置 | 来源 | 许可证 |
|---|---|---|
| `internal/wplauncher/` | [DHY0627/4399X19Login](https://github.com/DHY0627/4399X19Login) | MIT（原文保留在 `internal/wplauncher/LICENSE`） |
| `tools/frida/` | 本项目自写的逆向脚本 | 同本项目 |

### 思路与实现参考

- [Koud-Wind/Netease-minecraft-LAN-connects-to-Server](https://github.com/Koud-Wind/Netease-minecraft-LAN-connects-to-Server)：Java 版「房主开房 + 引流到服务器」同思路
- [GeyserMC/Geyser](https://github.com/GeyserMC/Geyser)、配套扩展 [DHY0627/GeyserNetease](https://github.com/DHY0627/GeyserNetease)（基于 [LoHJG/GeyserNetease](https://github.com/LoHJG/GeyserNetease)）
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
