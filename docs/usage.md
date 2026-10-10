# NeteaseBedrockGateway 使用文档

面向**部署者**的完整操作手册：从零把网关跑起来、让网易版原版客户端凭房间号进你的服务器，以及出问题怎么查。

> 只想尽快跑通？跳到 [1. 快速开始](#1-快速开始)。
> 想了解协议原理（为什么必须用配套扩展）？看 [README 的「工作原理」](../README.md#工作原理)。

---

## 目录

- [0. 它到底做了什么](#0-它到底做了什么)
- [1. 快速开始](#1-快速开始)
- [2. 前置准备](#2-前置准备)
- [3. 服务端侧：Geyser 与配套扩展](#3-服务端侧geyser-与配套扩展)
- [4. 网络与可达性](#4-网络与可达性)
- [5. 命令行参数逐个说明](#5-命令行参数逐个说明)
- [6. 常驻运行](#6-常驻运行)
- [7. 日常运维](#7-日常运维)
- [8. 端到端验收清单](#8-端到端验收清单)
- [9. 故障排查](#9-故障排查)
- [10. 安全与合规](#10-安全与合规)
- [附录 A：参数速查](#附录-a参数速查)
- [附录 B：日志速查](#附录-b日志速查)

---

## 0. 它到底做了什么

```
网易客户端                        网关（本程序）                     你的服务器
「本地联机」
  │ ① 输入房间号                                            ┌──────────────────────┐
  ├──────────► 网易 TanLobby 服务器 ◄──── ② 程序化建房 ──────│ Geyser + GeyserNetease│
  │                （房间号由网易分配）                      │ 扩展（RakNet UDP）    │
  │ ③ NetherNet（WebRTC/DTLS/SCTP）                          └──────────▲───────────┘
  └──────────────────────────────► 网关 ──── ④ 原样转发 Bedrock 帧 ──────┘
```

- **①②** 网关用你的 4399 账号登录、认证、创建房间，房间号由**网易服务器分配**（无法自选、无法固定）。
- **③** 玩家在网易客户端里输入房间号，客户端通过 NetherNet（WebRTC 数据通道）连到网关。
- **④** 网关把玩家与服务器之间的 Bedrock 帧**原样双向转发**，自己不解析游戏内容。

**关键结论**：网易链路是非标准的（RakNet 协议版本 8、自定义协议版本、不做 Bedrock 层加密），
所以**目标服务器必须是装了配套扩展的 Geyser**（见 [第 3 节](#3-服务端侧geyser-与配套扩展)），
标准 BDS / 原版 Geyser 都不行。

---

## 1. 快速开始

前提：你已经有一台**装了 Geyser + 配套扩展**的服务器（没有就先看 [第 3 节](#3-服务端侧geyser-与配套扩展)）。

**Windows**

```powershell
cd D:\gateway          # 建议单独建一个目录当工作目录
.\NeteaseBedrockGateway.exe -u 你的4399账号 -p 你的4399密码 -target 服务器IP:端口
```

**Linux / macOS**

```bash
mkdir -p ~/gateway && cd ~/gateway
./NeteaseBedrockGateway -u 你的4399账号 -p 你的4399密码 -target 服务器IP:端口
```

看到这一行就成功了 —— 把它打印的**房间号**告诉玩家：

```
[6/6] 房主网关运行中。房间号=123456，玩家流量将转发到 服务器IP:端口
[6/6] 请在网易客户端"本地联机"输入房间号 123456 加入
```

玩家侧：网易版 Minecraft → **本地联机** → **输入房间号** → 填 `123456` → 进服。

> 建议加 `-room-password "密码"`，否则知道房间号的人都能进。

---

## 2. 前置准备

| 项目 | 要求 | 说明 |
|---|---|---|
| 网关机器 | Windows / Linux / macOS，能访问公网 | 与服务器不必同机；同机最省事（延迟最低） |
| 4399 账号 | 一个可正常登录的账号 | 网关要**用它登录并建房**，属于「房主账号」；请用小号 |
| 目标服务器 | Geyser + 配套扩展，开放 RakNet（UDP）端口 | 见第 3 节 |
| 服务器地址 | `IP或域名:端口` | 端口即 Geyser 的 RakNet 端口 |
| Go 工具链 | 仅在**自己编译**时需要，Go 1.25+ | 也可直接下载预编译程序（见 README） |

**不需要**：公网 IP、端口映射、域名（见 [第 4 节](#4-网络与可达性)）。

---

## 3. 服务端侧：Geyser 与配套扩展

### 3.1 下载指定版本的 Geyser

网易链路只在特定 Geyser 版本上验证过，**建议直接用下面这个版本**，不要随手升到最新版。

**Geyser 2.10.1 · build 1170**（2026-06-19）—— 官方下载接口按「版本 + build + 平台」直连：

```
Velocity      https://download.geysermc.org/v2/projects/geyser/versions/2.10.1/builds/1174/downloads/velocity
Spigot/Paper  https://download.geysermc.org/v2/projects/geyser/versions/2.10.1/builds/1174/downloads/spigot
BungeeCord    https://download.geysermc.org/v2/projects/geyser/versions/2.10.1/builds/1174/downloads/bungeecord
Fabric        https://download.geysermc.org/v2/projects/geyser/versions/2.10.1/builds/1174/downloads/fabric
NeoForge      https://download.geysermc.org/v2/projects/geyser/versions/2.10.1/builds/1174/downloads/neoforge
Standalone    https://download.geysermc.org/v2/projects/geyser/versions/2.10.1/builds/1174/downloads/standalone
ViaProxy      https://download.geysermc.org/v2/projects/geyser/versions/2.10.1/builds/1174/downloads/viaproxy
```

### 3.2 装扩展

扩展必须放在 Geyser 的 **`extensions/` 子目录**（不是 `plugins/` 根目录）：

| 平台 | 路径 |
|---|---|
| Standalone | `extensions/GeyserNeteaseExtension.jar` |
| Velocity | `plugins/Geyser-Velocity/extensions/GeyserNeteaseExtension.jar` |
| BungeeCord | `plugins/Geyser-BungeeCord/extensions/GeyserNeteaseExtension.jar` |
| Spigot / Paper | `plugins/Geyser-Spigot/extensions/GeyserNeteaseExtension.jar` |

GeyserNetease建议使用[DHY0627/GeyserNetease](https://github.com/DHY0627/GeyserNetease)里的Release，已测试可以使用

### 3.3 必须设置 `ServerAddress`

必须在启动参数内加入：
```
-DGeyserNetease.ServerAddress=你的服务器IP或域名:端口
```

systemd 用户写进 `ExecStart=` 即可。

### 3.4 服务器参数要求

`server.properties` 设 `allow-flight=true` 并重启
`login-ratelimit 等设置为0` 并重启代理

### 3.5 建议的组合

`Geyser: 2.10.1-b1174`
`Velocity 3.5.1`

## 4. 网络与可达性

**谁连谁**：

| 连接 | 方向 | 协议 | 需要什么 |
|---|---|---|---|
| 网关 → 网易 | 出站 | TCP/UDP | 能上公网 |
| 网关 → 目标服务器 | 出站 | RakNet（UDP） | 能从网关机器访问 `-target` |
| 玩家 → 网关 | **由玩家发起** | NetherNet / WebRTC 数据通道 | 见下 |

网关建房时会向网易申请 **TURN 中继**（日志里能看到 `Resolved TURN server ...` 与
`Started refresh allocation timer`），玩家侧也会用网易的中继，因此：

- **通常不需要公网 IP、不需要端口映射**；玩家在公网上也能连进来（走中继，延迟略高）。
- **同一局域网/同一台机器**上会走 ICE 直连（日志里出现 `Channel binding successful: 192.168.x.x`），延迟最低。
- 网关机器只要有稳定的**出站**网络即可；如果所在网络禁止 UDP 出站，则可能连不上（换网络或放行）。

转发目标是**目标服务器的 RakNet 端口**（UDP）。填错端口/协议（例如填成 Java 端口 25565 TCP）会一直卡在
`[转发] 连接目标服务器第 N 次失败`。

---

## 5. 命令行参数逐个说明

```
NeteaseBedrockGateway -u <账号> -p <密码> -target <服务器:端口> [可选参数...]
```

### 5.1 必填

| 参数 | 说明 |
|---|---|
| `-u` | 4399 用户名（房主账号） |
| `-p` | 4399 密码 |
| `-target` | 玩家流量**转发到哪**，必须是 `IP/域名:端口` 形式的 Geyser RakNet 端口 |

三个缺任何一个都会打印用法并以退出码 `2` 退出。

### 5.2 两个地址别搞混

| 参数 | 含义 | 谁去连它 | 默认 |
|---|---|---|---|
| `-target` | 玩家流量**转发到哪**（你的 Geyser） | **网关**去连 | 必填 |
| `-server-address` | 玩家**去哪找房主**（NetherNet 入口） | **玩家**去连 | 留空 = 继承 `-target` |

绝大多数部署里二者相同（网关与 Geyser 同机、共用同一个 RakNet 端口），**不用写**。
只有当网关与服务器不在同一处（例如网关单独挂了一台中转机）时才需要显式指定。
启动日志会同时打印这两个值，建议对着确认一遍。

### 5.3 房间外观与准入

| 参数 | 默认 | 说明 |
|---|---|---|
| `-room-name` | `NeteaseBedrockGateway Host Room` | 房间名称（对实际进服没影响） |
| `-capacity` | `8` | 房间容量 |
| `-room-password` | 空 | 房间密码；留空 = 无密码，知道房间号即可进。**公网环境建议设置** |

### 5.4 版本标识（**最容易踩的地方**）

| 参数 | 默认 | 说明 |
|---|---|---|
| `-level-id` | `r6YhQdny6LU=` | 房间游戏版本标识（base64 的 8 字节）。⚠️ **不要改成空**，见下 |
| `-version-string` | `1.21.120.0` | 玩家进房时校验的版本字符串 |
| `-map-id` | `0` | 房间 MapID（版本标识） |
| `-protocol-id` | `42` | 房间 ProtocolID（`42` 与原版房间一致） |
| `-game-type` | `0` | 房间 GameType |

> ⚠️ **`-level-id` 留空 = 玩家能进房间但永远卡在「等待房主开始游戏」。**
>
> 症状链：日志里有 `★ 新玩家加入房间`、有 `已向玩家上报 NetherNetID=...`，
> 但**永远没有** `handleOffer`、**没有 ICE 日志**、**没有** `★ 收到玩家连接`。
> 客户端拿不到有效版本标识就不会去建立 NetherNet 连接。
>
> 判据：**看有没有 `handleOffer`**。补回该值后同一条链路上会立刻出现：
>
> ```
> ★ 新玩家加入房间 → handleOffer: raw signal data ... → DTLS handshake completed → ★ 收到玩家连接
> ```
>
> 顺带一提：日志里的 `ErrorCode=-74 玩家数=0` 是**无害噪音**，成功跑通时照样出现。
>
> `-map-id` / `-protocol-id` / `-game-type` 一般**不要动**，只有在玩家客户端大版本变化、
> 且确认房间进不去时才需要跟着调整（取值来自逆向真实客户端的建房报文）。

### 5.5 落盘与保活

| 参数 | 默认 | 说明 |
|---|---|---|
| `-room-file` | `room.json` | 房间信息落盘文件；同时写一个同名 `.txt`（只存房间号）。**空字符串 = 不落盘** |
| `-keepalive` | `25s` | 房间存活检查间隔；`0` = 关闭。**连续 3 次查不到即判定房间被回收并自动重建** |

`room.json` 内容（外部脚本可读，用来给玩家播报房间号）：

```json
{
  "room_id": 123456,
  "room_name": "NeteaseBedrockGateway Host Room",
  "target": "服务器IP:端口",
  "host_nether_id": "1234567890123456789",
  "status": "alive",
  "created_at": "2026-01-01T00:00:00+08:00",
  "updated_at": "2026-01-01T00:05:00+08:00",
  "recreations": 0
}
```

`room.txt` 就是纯房间号一行，例如 `cat room.txt` → `123456`。

### 5.6 诊断开关

| 参数 | 默认 | 说明 |
|---|---|---|
| `-nether-msg-limit` | `0` | **仅排查用**。转发给玩家的单条 NetherNet 消息上限（字节）。设成 `262144` 会把超过客户端 `max-message-size` 的帧**丢弃** —— 能压住某些闪退，但**会丢区块**，别在正常运营时开 |

查看全部参数：`NeteaseBedrockGateway -h`。

---

## 6. 常驻运行

**先记住一件事**：`room.json` / `room.txt` / `relay.log` 都写在**当前工作目录**，
所以务必**先 `cd` 到一个专用目录**再启动；用 systemd 就配 `WorkingDirectory=`。

### 6.1 Linux（systemd）

`/etc/systemd/system/netease-gateway.service`：

```ini
[Unit]
Description=NeteaseBedrockGateway
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=youruser
WorkingDirectory=/home/youruser/gateway
ExecStart=/home/youruser/gateway/NeteaseBedrockGateway -u 你的4399账号 -p 你的4399密码 -target 服务器IP:端口 (-room-password "房间密码")
Restart=always
RestartSec=10
StandardOutput=append:/home/youruser/gateway/host.log
StandardError=append:/home/youruser/gateway/host.log

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now netease-gateway
journalctl -u netease-gateway -f        # 或 tail -f ~/gateway/host.log
```

> ⚠️ 密码写在 `ExecStart` 里，任何能读该 unit 文件的用户都能看到。
> 建议 `sudo chmod 600 /etc/systemd/system/netease-gateway.service`。

网关收到 `SIGTERM`/`SIGINT`（`systemctl stop`、`Ctrl+C`）会**优雅退出**，不会留下脏状态。

### 6.2 Linux（screen / tmux，临时用）

```bash
screen -dmS gateway bash -c 'cd ~/gateway && exec ./NeteaseBedrockGateway -u 账号 -p 密码 -target 服务器IP:49780 > host.log 2>&1'
screen -r gateway        # 查看
```

### 6.3 Windows

```powershell
# 前台运行（关掉窗口就停）
cd D:\gateway
.\NeteaseBedrockGateway.exe -u 账号 -p 密码 -target 服务器IP:端口

# 后台 + 落日志
Start-Process -FilePath .\NeteaseBedrockGateway.exe `
  -ArgumentList '-u','账号','-p','密码','-target','服务器IP:端口' `
  -WorkingDirectory D:\gateway -RedirectStandardOutput D:\gateway\host.log
```

要开机自启，用「任务计划程序」（触发器=登录时/启动时，「起始于」填 `D:\gateway`），
或用 [NSSM](https://nssm.cc/) 注册成 Windows 服务。

### 6.4 多开（多个房间）

每个实例要**独立的账号 + 独立的工作目录**（或至少不同的 `-room-file` 文件名）：

```bash
cd ~/gw-room1 && ./NeteaseBedrockGateway -u 账号A -p ... -target ... -room-file room1.json
cd ~/gw-room2 && ./NeteaseBedrockGateway -u 账号B -p ... -target ... -room-file room2.json
```

同类网易账号频繁建房可能触发风控，多开请谨慎。

---

## 7. 日常运维

### 7.1 房间号怎么给玩家

三种方式，任选：

```bash
cat room.txt                                  # 最省事，纯数字
jq .room_id room.json                         # 需要 jq
grep -o '房间号=[0-9]*' host.log | tail -1     # 从日志捞
```

> 房间号**每次重启/重建都会变**，不要写死在公告里；建议做成玩家可查询的命令（读 `room.txt`）。

### 7.2 房间被回收怎么办

网易会回收长时间无人的房间。网关每 `-keepalive`（默认 25 秒）查一次，**连续 3 次查不到**就自动重建，
日志形如：

```
[房主] 房间 123456 存活（在线 0 人，累计 3 人，已运行 20m2s，重建 0 次）
[房主] 房间存活检查失败（1/3）: ...
[房主] 房间 123456 失效（...），准备重建 ...
[3/6] 连接中转服务器并创建房间 ...（第 2 次尝试）
[3/6] ★ 房间创建成功 RoomID=654321
```

重建后 `room.json` 的 `recreations` 会 +1，房间号变化 —— **记得重新广播给玩家**。

### 7.3 4399 登录相关

| 日志 | 含义 | 处理 |
|---|---|---|
| `[1/6] 认证成功: uid=...` | 正常 | —— |
| `开房失败: ...（15 秒后重试）` | 建房请求被拒 | 等它自己重试；持续失败检查账号是否被封/风控 |
| `重新登录 4399 失败: ...（60 秒后重试，登录有限频）` | 凭据过期后重登被限频 | **等 1–3 分钟**，不要反复重启（越重启越容易被限） |
| `lookup ...: no such host` | DNS 失败 | 检查网关机器 DNS/网络，稍后自动重试 |
| `获取开房凭据失败（第 N 次）` | 凭据接口失败 | 每 10 秒重试，通常自愈 |

### 7.4 升级

```bash
systemctl stop netease-gateway
# 替换可执行文件（Windows 直接覆盖 .exe）
systemctl start netease-gateway
```

房间号会变，其余无需调整；`room.json`/`room.txt` 会被新实例覆盖。

### 7.5 日志与诊断

| 文件 | 内容 | 何时产生 |
|---|---|---|
| 标准输出（建议重定向 `host.log`） | 主日志：登录/建房/玩家/保活 | 一直 |
| `room.json` / `room.txt` | 当前房间信息 | 每次状态变化 |
| `relay.log` | **每一次转发的完整 hex**（`[玩家→服务器] N: ...`、`[服务器→玩家] N: ...`） | 有玩家流量时 |

`relay.log` 会一直增长，长期运营建议定期清理。排查时用 `cmd/diag/relaydecode` 把 hex 解成包列表：

```bash
go run ./cmd/diag/relaydecode relay.log
```

README 的[诊断工具](../README.md#诊断工具)一节还有几十个独立小工具（抓包、模拟登录、解码等）。

---

## 8. 端到端验收清单

按日志顺序对一遍，全程应无缺环：

| # | 位置 | 应该看到 | 缺了说明 |
|---|---|---|---|
| 1 | 网关 | `[1/6] 认证成功`、`[2/6] 凭据就绪` | 4399 账号问题/限频 |
| 2 | 网关 | `[3/6] ★ 房间创建成功 RoomID=...` | 建房失败，等重试 |
| 3 | 网关 | `[6/6] 房主网关运行中。房间号=...` | —— |
| 4 | 网关 | 玩家进房时 `★ 新玩家加入房间` + `已向玩家上报 NetherNetID=` | 客户端没连上房间 |
| 5 | 网关 | **`handleOffer`** → `DTLS handshake completed` → `★ 收到玩家连接` | 卡「等待房主开始游戏」→ 查 `-level-id` |
| 6 | 网关 | `[转发] 已连接目标服务器: ...` | `-target` 写错/端口不通（会看到 `连接目标服务器第 N 次失败`） |
| 7 | 网关 | `[转发] 玩家→服务器 N 字节` / `服务器→玩家 N 字节` | 扩展没装或不认网易协议 |
| 8 | 服务端 | Geyser 日志出现玩家进服；后端日志 `logged in with entity id ...` / `joined the game` | 见第 9 节 |

---

## 9. 故障排查

| 症状 | 先看哪里 | 多半是 |
|---|---|---|
| 卡在「等待房主开始游戏」；有 `★ 新玩家加入房间`，但**没有 `handleOffer`** | 启动参数 | **`-level-id` 留空**（本项目第一大坑） |
| 同上，但 `-level-id` 确认非空 | 启动日志 `转发目标` / `上报房主地址` | `-server-address` / `-target` 填错 |
| 玩家进房后**零数据**、约 90 秒超时、`relay.log` 不生成 | 网关日志 `★ 收到玩家连接` 之后 | 依赖 `nemc-tan-lobby-solver` 的补丁没打（见 README 依赖一节） |
| 客户端显示 **「数据流终止」**，Geyser 同款，代理无日志 | 扩展的 `-DGeyserNetease.Sniff=true` | Geyser 的 java 握手 hostname 为空 → 设 `-DGeyserNetease.ServerAddress` |
| 客户端进房后**一直卡住不进服**：Geyser 发完 `ServerToClientHandshake` 后再无玩家数据，约 30 秒超时 | 扩展是不是上游原版源码编的 | 上游**无条件**做加密握手，网易客户端对该包静默忽略 → 打 [patches](../patches/README.md) 补丁或用 fork jar |
| 一直 `连接目标服务器第 N 次失败` | `-target` | 端口/协议不对（要 Geyser 的 **UDP** RakNet 端口） |
| 玩家进去后被踢，`multiplayer.disconnect.flying` | 后端 `server.properties` | `allow-flight=false` → 改 `true` |
| Velocity 报「登录过于频繁」 | `velocity.toml` | `login-ratelimit = 0` |
| 扩展被拒：`API 版本错误，当前版本为：x.y.z` | 扩展的 `extension.yml` | `api:` 高于 Geyser 版本 → 下调后重新编译 |
| 房间突然消失 / 房间号变了 | 网关日志有没有 `失效（...），准备重建` | 房间被网易回收，属正常；重新广播新房间号 |
| 启动即报 4399 登录失败 | —— | 账号密码错误，或触发登录限频（等 1–3 分钟） |

更完整的排查方法与协议细节见 [docs/troubleshooting.md](troubleshooting.md)。

---

## 10. 安全与合规

- **房间号 ≈ 门票**：默认无密码，知道号就能进。公网使用请加 `-room-password`，并只把房间号发给信任的人。
- **4399 账号**：网关需要你的账号密码来登录并建房。用**小号**，不要把账号密码写进公开脚本/仓库；unit 文件权限收紧（`600`）。
- **房间不可固定**：房间号由网易分配，网关重启/重建后会变，无法做到「固定房间号」。
- **风控**：短时间内反复重启、频繁建房、多账号同机操作都可能触发网易侧限频或风控。
- **用途限制**：本项目通过逆向与协议模拟实现，**与网易官方无关**；请遵守相关服务条款，风险自负（见 README 免责声明）。

---

## 附录 A：参数速查

| 参数 | 默认 | 必填 |
|---|---|---|
| `-u` | — | ✅ |
| `-p` | — | ✅ |
| `-target` | — | ✅ |
| `-server-address` | 继承 `-target` | 否 |
| `-room-name` | `NeteaseBedrockGateway Host Room` | 否 |
| `-capacity` | `8` | 否 |
| `-room-password` | 空 | 否 |
| `-level-id` | `r6YhQdny6LU=` | 否 |
| `-version-string` | `1.21.120.0` | 否 |
| `-map-id` | `0` | 否 |
| `-protocol-id` | `42` | 否 |
| `-game-type` | `0` | 否 |
| `-room-file` | `room.json` | 否 |
| `-keepalive` | `25s` | 否（`0` = 关闭） |
| `-nether-msg-limit` | `0` | 否（仅排查用） |

## 附录 B：日志速查

| 日志片段 | 含义 |
|---|---|
| `[1/6] 4399 登录 + x19 认证` | 开始登录 |
| `[2/6] 凭据就绪: raknet=... signaling=...` | 拿到开房凭据 |
| `[3/6] ★ 房间创建成功 RoomID=123456` | **房间号** |
| `[4/6] 房间可查询` | 保活前置检查通过 |
| `[6/6] 房主网关运行中` | 开始接受玩家 |
| `★ 新玩家加入房间（ErrorCode=-74 玩家数=0）` | 有玩家进房; |
| `handleOffer: parsed description` | 玩家开始 WebRTC 协商 |
| `DTLS handshake completed successfully` | 数据通道加密完成 |
| `★ 收到玩家连接` | 玩家已连上网关 |
| `[转发] 已连接目标服务器: ...` | 网关连上了你的 Geyser |
| `[转发] 玩家→服务器 N 字节` / `服务器→玩家 N 字节` | 正在双向转发 |
| `[房主] 房间 123456 存活（在线 N 人，累计 M 人，已运行 ...，重建 K 次）` | 每 25 秒一次保活心跳 |
| `[房主] 房间 N 失效（...），准备重建 ...` 后接 `[3/6] ★ 房间创建成功 RoomID=...` | 旧房间被回收，已自动重建 |

---

相关链接：[README](../README.md) ·
[故障排查](troubleshooting.md) ·
[Geyser 2.9/2.10 兼容补丁](../patches/README.md) ·
[扩展 fork](https://github.com/DHY0627/GeyserNetease) ·
[上游扩展](https://github.com/LoHJG/GeyserNetease)
