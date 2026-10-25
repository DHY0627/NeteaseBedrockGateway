# Geyser 2.9 / 2.10 兼容补丁（GeyserNetease）

配套 fork [DHY0627/GeyserNetease](https://github.com/DHY0627/GeyserNetease) 的 **`main` 分支已切换为 Geyser 2.9.4 兼容版本**（`api: 2.9.4`），`dist/` 里的预编译 jar 直接就能用在 2.9.4 ~ 2.10.1 上。之前基于 Geyser 2.11.3 的源码保留在该仓库的 `2.11.3` 分支（含 `org.geysermc.geyser.network.bedrock.raknet.*`、`BedrockPingHandler`、`RaknetServer` 等 2.11 专用类），**不再发布新构建**。

本目录的补丁则把这套扩展的**协议行为**移植到上游源码 [LoHJG/GeyserNetease](https://github.com/LoHJG/GeyserNetease) 上，方便从上游自行构建。

- 文件：`geysernetease-geyser29-compat.patch`
- 上游基线：`2987a328062b0eef0d78de40d8b0a37955615903`（2026-07-24 `Delete lib directory`）
- 改动规模：1 个文件、+147 / −4 行
- 只动 `NetEaseUpstreamHandler.java`：**只改协议行为，不碰版本相关的接线层**（`ServerRestartUtil` / initializer / RakNet 类路径一律保持上游原样，所以补丁本身与 Geyser 版本无关）

> ✅ 已验证：在**全新克隆**的上游仓库上 `git apply --check` 通过、`git apply` 后 `./gradlew shadowJar` 构建成功（产物约 3.1 MB）；
> 该产物在 **Geyser 2.10.1-b1174 + Velocity 3.5.1** 上由网易原版客户端**成功进服**（后端日志可见 `logged in with entity id ... / joined the game`）。

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

| 修复 | fork（现在是 2.9.4 版） | 上游 + 本补丁（2.9.x / 2.10.x） |
|---|---|---|
| 跳过 `ServerToClientHandshake` | 已有（`SKIP_ENCRYPTION`，默认开） | 本补丁提供 |
| `ServerAddress` 修正 | 已有（`patchServerAddress`） | 本补丁提供 |
| Java 侧嗅探 | 已有（`-DGeyserNetease.Sniff`） | 本补丁提供 |
| ConfirmSkin 皮肤确认 | 已有（2026-10-17 修复） | 上游源码同步修复 |
| `geyser` 命令表过滤 | 已有（2026-10-17 修复） | 上游源码同步修复 |
| RakNet / 接线层 | 2.11 专用类路径 | **保持上游原样**，未改动 |

补丁不含版本相关改动，因此**不需要**跟随 Geyser 升级重写接线层；升级 Geyser 时通常只需确认 `extension.yml` 的 `api:`。

---

## 预编译产物（不想自己编译就用这个）

`dist/` 里放了**已经编译好、且实测进服成功**的扩展，省去装 JDK 和 Gradle：

| 文件 | 大小 | SHA256 |
|---|---|---|
| `dist/GeyserNeteaseExtension-2.9.4-api.jar` | 11,517,133 B | `E6212C4E533632909F020CCF2F7B5D37C6AA1F4425B5A896C5FA2E5BB6EF3504` |
| `dist/geysernetease-geyser29-compat.patch` | 10,669 B | `23F5EF6B521182FDC1C2B2F75C8C6EF0A752E0C2D00CB8D7DE8435F2CF1B498A` |

构建来源：上游 `2987a328062b0eef0d78de40d8b0a37955615903` + 本目录补丁 +
`extension.yml` 的 `api:` 改为 `2.9.4`，**并同步修复了 `SpoofedUpstream` 的皮肤确认与命令表问题**（见下节）。

> **2026-10-25 起：只发布 Geyser 2.9.4 兼容版本。**
> fork（`DHY0627/GeyserNetease`）的 `main` 分支已整体切换为 2.9.4 版
> （`api: 2.9.4`、Java 17、`geyser-core 2.9.5-SNAPSHOT` / `bedrock-codec 3.0.0.Beta12`），
> CI 之后发布的每个 Release 都是 **2.9.4 兼容构建**。
> 之前基于 Geyser 2.11.3 的源码保留在 fork 的 `2.11.3` 分支，**不再发布新构建**，
> 因此 `dist/` 里也移除了 `GeyserNeteaseExtension-2.11.3-api.jar`。

> 2026-10-17 更新：修复网易客户端两个已知问题（与 fork 分支同步）。
> - **其他玩家自定义皮肤进入超时**：`ConfirmSkinSerializer` 对每条 entry 的
>   `uuid` / `skinBytes` / `geoStr` 都会 `checkNotNull`，而原实现在 `StartGame` 时只填
>   `valid + uidStr`，`uuid` 等字段为 null，编码阶段必然抛异常，客户端等不到皮肤确认而超时。
>   现改为：删除空占位包，`PlayerSkin` / `PlayerList` 统一走 `sendSkinCsp`；真实皮肤发
>   `valid=true` + 完整 `image`/`geo`，Persona / 4D 皮肤发 `valid=false` 空条目回退默认皮肤。
> - **点击屏幕出现 `geyser:xxx` 提示**：Geyser 扩展命令（`geyser` 根命令）会进入下发的
>   `AvailableCommandsPacket`，网易客户端点击时把它渲染成 `geyser:xxx`。现改为在下发前从
>   命令表中剔除 `geyser` 根命令，Java 服务器的原生命令保持不变。
> - **点击屏幕出现 `geyseropt:cooldown:non` 提示**：Geyser 的 `GeyserSession#sendJsonUIData`
>   用 TIP 类型 `TextPacket` 传 JSON UI 数据（`geyseropt:cooldown:*`），依赖集成资源包的
>   `hud_screen.json` 解析成冷却指示器；网易客户端不认这套 JSON UI，会把文本原样显示。
>   现在 `SpoofedUpstream` 在下发前丢弃所有以 `geyseropt:` 开头的 TIP 文本。
>
> **聊天问题（诊断中）**：网易客户端 CHAT/POPUP 包在标准字段后还有一个额外字符串字段，
> 其语义尚未确认，目前仍按历史行为写空串。若实测聊天不显示，可加
> `-DGeyserNetease.TextExtraField=message` 让扩展改写消息原文来验证该字段作用；
> 同时打开 `-DGeyserNetease.Debug=true` 会在控制台打印收到的每个 `TextPacket`
> （type / message / filtered / sourceName / xuid），便于定位。

> 2026-10-22 更新：畸形数据包加固（两个 jar 均已重新构建）。
> 对客户端可控的长度 / 计数 / 索引全部加上限或校验，避免被构造包打成 OOM 或
> 未捕获异常断连：
> - `NeteaseCodecRegistry` 的 `EncodingSettings` 恢复上游默认上限
>   （列表 1536 / 字节数组 1MB / 网络 NBT 512MB / 物品 NBT 100KB / 字符串 32KB），
>   原先全部是 `Integer.MAX_VALUE`，等于关闭了 bedrock-codec 的畸形包防护。
> - `BiomeDefinitionList`（819/860）tag 数量上限 4096，并修掉 `length * 2` 的整数溢出
>   （溢出为负会让 `isReadable` 检查失效，随后按恶意长度预分配 → 远程 OOM）。
> - `ConfirmSkin` 条目上限 64、`PlayerEnchantOptions` 自定义附魔上限 256。
> - `PlayerAuthInput`（630/686/766）的 `inputMode` / `playMode` 索引越界改为受控异常。
> - `InventoryTransaction` 的 `source` / `toItem` / 手持物品判空；方块哈希表构建异常隔离。
> - 资源包 `chunkIndex` 负值 / 越界校验（原先 `channel.position(负值)` 会抛未捕获异常）。
> - `LoginEncryptionUtils` 与表单响应包加 try/catch，畸形包断开或记录后忽略。

> 2026-10-25 更新：修复「进服即被踢」的 `getPacketDefinition` NPE（两个 jar 均已重新构建）。
> - 现象：`io.netty.handler.codec.EncoderException: java.lang.NullPointerException: Cannot invoke
>   "BedrockPacketDefinition.getId()" because the return value of "BedrockCodec.getPacketDefinition(Class)"
>   is null`，随后 `已因 An internal error occurred! 与远程 Java 服务器断开了连接`。
> - 原因：扩展的 `SpoofedUpstream` 把 `getProtocolVersion()` 覆写成 Geyser 的
>   `GameProtocol.DEFAULT_BEDROCK_PROTOCOL`。Geyser **2.11.3** 的默认协议已到 **v2193**
>   （MC 26.50+），而本扩展的网易 codec 最高只到 **v860**；Geyser 于是按 v2193 发包，
>   v870+ 才有的包类在 v860 codec 里没有注册 → 编码阶段 `getPacketDefinition` 返回 null → 踢人。
> - 修复：删除该覆写，回归基类行为（返回会话真实 codec 版本）；另加 `skipUnknownPacket`
>   双保险，遇到当前 codec 未注册的包只警告一次并跳过，不再断连。
> - 注意：`NetEaseUpstreamHandler` 里用于选取方块/物品注册表的 `spoofedVersion` 仍是
>   `DEFAULT_BEDROCK_PROTOCOL`（需要最新方块表做哈希），与上面的协议版本是两回事。

> 2026-10-25 更新（二）：新增**调试日志加密上传**（两个 jar 均已重新构建，体积从 ~3.1 MB 增至 ~11.5 MB）。
> - 打开 debug（`-DGeyserNetease.Debug=true` 或 `debug-mode: true`）后：Geyser 启动时从
>   `cdn.redstone.xin/key/` 拉取 **ML-KEM-1024** 公钥；结束时把本次日志 GZIP 压缩，
>   用 ML-KEM 封装出的共享密钥做 **encrypt-then-MAC**（XOR 密钥流 + HMAC-SHA256）加密，
>   打包成 `<10位id>.zlog` 上传到 `log.4f4t.top`，上传完再退出。
> - `.zlog` 载荷与 `ML-KEM-完整技术文档.md` 4.2 / 5.5 节逐字节一致：
>   `kem_ct(1568) ‖ nonce(16) ‖ tag(32) ‖ ct'`，服务端只需 `private.key` 即可解封，
>   完整 Python 解密脚本见 GeyserNetease 仓库的 `docs/ZLOG.md`。
> - `kem.enc` 仅作为 CDN 密钥文件可用性探测，**不参与加密**（客户端没有私钥，解不开它）。
> - 体积变大是因为 shadowJar 内 relocate 了 BouncyCastle（JDK 21 没有内置 ML-KEM）。
> - 可用系统属性覆盖：`GeyserNetease.ZLogUploadUrl`、`GeyserNetease.ZLogApiKey`、
>   `GeyserNetease.KemPublicKeyUrl`、`GeyserNetease.KemEncUrl`、`GeyserNetease.ZLogFile`、
>   `GeyserNetease.ZLogMaxBytes`。


**实测环境**：Geyser-Velocity **2.10.1 build 1174**（`git-master-7b60de1`）+ Velocity 3.5.1 + 网易版客户端。

部署位置：`plugins/Geyser-Velocity/extensions/GeyserNeteaseExtension.jar`
（Standalone 则是 `extensions/GeyserNeteaseExtension.jar`）。

启动后必须能在控制台看到扩展加载成功的日志，否则网易链路不会生效：

```
已启用扩展 GeyserNetEase
NetEase Extension initialized — RakNet v8 clients supported.
```

> 给别人部署时注意：**`api:` 必须 ≤ 对方 Geyser 的 API 版本**。
> 2.9.4 这个值在 2.9.4 ~ 2.10.1 上都能加载；若对方是 2.11.x，请自行从 fork 的 `2.11.3` 分支构建（该分支不再发布构建产物）。
