# Geyser 2.9 / 2.10 兼容补丁（GeyserNetease）

配套 fork [DHY0627/GeyserNetease](https://github.com/DHY0627/GeyserNetease) 适配的是 **Geyser 2.11.3**，用到的 `org.geysermc.geyser.network.bedrock.raknet.*`、`BedrockPingHandler`、`RaknetServer` 这些类在 **Geyser 2.9.x / 2.10.x 上不存在**，那份源码根本编译不过。

本目录的补丁把这套扩展的**协议行为**移植到能在 2.9.x / 2.10.x 上编译的上游源码 [LoHJG/GeyserNetease](https://github.com/LoHJG/GeyserNetease) 上。

- 文件：`geysernetease-geyser29-compat.patch`
- 上游基线：`2987a328062b0eef0d78de40d8b0a37955615903`（2026-07-24 `Delete lib directory`）
- 改动规模：1 个文件、+147 / −4 行
- 只动 `NetEaseUpstreamHandler.java`：**只改协议行为，不碰版本相关的接线层**（`ServerRestartUtil` / initializer / RakNet 类路径一律保持上游原样，所以补丁本身与 Geyser 版本无关）

---

## 为什么需要它

在 Geyser 2.10.1-b1174 + Velocity 3.5.1 上实测，网易客户端在 `Geyser → 代理` 这一跳会卡死。原因是**两个独立的问题叠在一起**：

### 1. 网易客户端收到 `ServerToClientHandshake` 会静默忽略并超时

上游 `handle(LoginPacket)` **无条件**执行加密握手，然后 `loginDeferred = true` 死等客户端的 `ClientToServerHandshake`：

```java
startEncryptionHandshake(session, result.identityClaims().parsedIdentityPublicKey());
neteaseSession.setLoginDeferred(true);   // ← 永远等不到
```

而网易局域网客户端对这个包**静默忽略**，从不回 `ClientToServerHandshake`。现象：

```
服务器→玩家 14 B  └ 包 ID=143 NetworkSettings
玩家→服务器 8394 B（Login）
服务器→玩家 359 B └ 包 ID=3 ServerToClientHandshake
（此后玩家侧一个字节都没有，约 30 秒后超时断开）
```

> 注意：这里**与 Geyser 版本无关**。换 2.9.4 / 2.9.5 / 2.10.1 现象完全一样。
> 另外，出现 `ServerToClientHandshake` **不能**用来判断「走的是标准路径」—— 网易路径自己就会发这个包。

补丁改为跳过握手，并**自己补发**本该由 `ClientToServerHandshake` 分支下发的两个包：

```java
neteaseSession.setLoginDeferred(false);
session.sendUpstreamPacket(PlayStatus(LOGIN_SUCCESS));
session.sendUpstreamPacket(ResourcePacksInfo);
```

### 2. `ServerAddress = ":0"` 导致 Java 握手 hostname 为空 → 被代理静默掐断

网易**局域网**客户端上报的客户端数据里 `ServerAddress` 就是字符串 `":0"`：

```
[geyser-netease] 客户端 ServerAddress=":0"
```

而 Geyser 的 `forward-hostname` 为 `true` 时，Java 握手用的 hostname 来自 `joinAddress()`：

```
joinAddress() = substring(0, lastIndexOf(':')) = substring(0, 0) = ""   // 空字符串
```

于是 Geyser 带着**空 hostname** 去连 Velocity，Velocity 在登录阶段**直接静默掐断** ——
`InitialLoginSessionHandler` / `HandshakeSessionHandler` 这类"前线"处理器不打任何日志，代理日志里一片空白，只留下 Geyser 侧一句：

```
已连接到 Java 服务器
已因 数据流终止 与 Java 服务器断开了连接
```

补丁在客户端数据就绪后把非法值修正为一个真实地址：

```java
patchServerAddress(session.getClientData());   // ":0" / 空 / 含 '|' → FORCED_SERVER_ADDRESS
```

默认值 `example.com:19132` 是**脱敏占位值，必须替换成你自己的域名/IP**，可以改代码，也可以启动时覆盖：

```
-DGeyserNetease.ServerAddress=你的域名:19132
```

### 修复后的完整链路（实测）

```
客户端 ServerAddress=":0" → 已修正为 "example.com:19132"
已跳过 ServerToClientHandshake
已下发 PlayStatus(LOGIN_SUCCESS) + ResourcePacksInfo
出-> ClientIntentionPacket(hostname=example.com, port=8081, intent=LOGIN)
出-> ServerboundHelloPacket(username=...)
入<- ClientboundLoginFinishedPacket(...)
后端服: <玩家> logged in with entity id 1200 — joined the game
```

---

## 用法

```bash
# 1) 拉上游源码
git clone https://github.com/LoHJG/GeyserNetease.git
cd GeyserNetease

# 2) 打补丁
git apply /path/to/geysernetease-geyser29-compat.patch
#    上游 master 已前移导致冲突时：git apply -3 ...

# 3) 构建（需要 JDK 17+）
./gradlew shadowJar          # 产物：build/libs/GeyserNeteaseExtension.jar
```

**注意 `src/main/resources/extension.yml` 里的 `api:` 必须 ≤ 你 Geyser 的 API 版本**，否则扩展会被拒绝加载：

```
无法加载扩展 GeyserNetEase：API 版本错误，当前版本为：2.9.10
```

上游默认是 `api: 2.9.10` —— 在 2.9.10+ 和 2.10.x 上可以直接用；如果你跑的是 2.9.4 这类更老的版本，把它调低（例如 `2.9.4`）后重新构建。

### 部署位置

| 平台 | 路径 |
|---|---|
| Standalone | `extensions/GeyserNeteaseExtension.jar` |
| Velocity | `plugins/Geyser-Velocity/extensions/GeyserNeteaseExtension.jar` |
| BungeeCord | `plugins/Geyser-BungeeCord/extensions/GeyserNeteaseExtension.jar` |
| Spigot/Paper | `plugins/Geyser-Spigot/extensions/GeyserNeteaseExtension.jar` |

---

## 诊断开关

| 开关 | 默认 | 作用 |
|---|---|---|
| `-DGeyserNetease.ServerAddress=host:port` | `example.com:19132` | 覆盖握手用的强制地址 |
| `-DGeyserNetease.Sniff=true` | `false` | 在 Geyser→Java 会话上挂监听器，把**每一个 Java 包的收发、断开原因**打到控制台（前 150 行）并追加写入运行目录的 `geyser-netease-java.log` |

排查「Java 侧被静默断流」这类问题时打开 `Sniff` 最有效，它能把代理那侧看不到的东西全部打出来：

```
[JSniff] 诊断: forwardHostname=true joinAddress=example.com remoteServer=... clientData.ServerAddress=example.com:19132
[JSniff] 出-> ClientIntentionPacket(protocolVersion=775, hostname=example.com, port=8081, intent=LOGIN)
[JSniff] == 正在断开: reason=... cause=...
```

正常游玩时建议关闭（它会打印大量日志并持续写文件）。

---

## 与 fork 的关系

| 修复 | fork（2.11.3） | 上游 + 本补丁（2.9.x / 2.10.x） |
|---|---|---|
| 跳过 `ServerToClientHandshake` | 已有（`SKIP_ENCRYPTION`，默认开） | 本补丁提供 |
| `ServerAddress` 修正 | 已有（`patchServerAddress`） | 本补丁提供 |
| Java 侧嗅探 | 已有（`-DGeyserNetease.Sniff`） | 本补丁提供 |
| RakNet / 接线层 | 2.11 专用类路径 | **保持上游原样**，未改动 |

补丁不含版本相关改动，因此**不需要**跟随 Geyser 升级重写接线层；升级 Geyser 时通常只需确认 `extension.yml` 的 `api:`。
