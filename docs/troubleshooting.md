# 排错记录：网易客户端「进不去 Java 服」的两个静默失败

> 2026-10-06 · 现象从「连接超时」到「数据流终止」，最后定位到两处**完全不报错**的失败点。
> 这份记录既是复盘，也是排查同类问题的操作手册。

## 现象时间线

| 阶段 | 客户端表现 | 服务端表现 |
|---|---|---|
| 1 | 「等待房主开始游戏」 | 房间创建成功、可查询，但玩家进房被拒 |
| 2 | 进房后「连接超时」（90 秒） | 网关日志显示玩家 SCTP 已建立，但**一个字节都没收到**，`relay.log` 不生成 |
| 3 | 「数据流终止」 | Geyser：`已因 §r数据流终止 与 Java 服务器断开了连接`；**Velocity 一行日志都没有** |

---

## 根因 ①：NetherNet 库丢掉客户端的**第一个** Bedrock 包

### 证据

网关（Go）侧 pion 的 SCTP 日志：

```
[pion] accepted a new stream (streamIdentifier: 1)
[pion] [1:...] reassemblyQueue readable=true      ← 出现两次
[pion] [1:...] readNotifier.signal()
[pion] [1:...] reassemblyQueue readable=true
[pion] [1:...] readNotifier.signal()
[pion] [3:...] reliability params: ordered=false type=1 value=0
[pion] stats nDATAs (in) : 3                      ← 客户端总共只发了 3 个 DATA
```

`nDATAs=3` = stream 1 上 2 个 + stream 3 上 1 个。其中两个是 DCEP 握手（`reliability params` 只打印了两次 = 两个通道的 OPEN），**stream 1 上的第 2 个 DATA 再也没出现过**——而它正是客户端的 `RequestNetworkSettings`（`06 c1 01 00 00 03 5c`）。

### 根因

库的代码路径（`nemc-tan-lobby-solver`）：

```go
// core/webrtc/datachannel.go：通道一打开就开始读
d.readLoopActive = make(chan struct{})
go d.readLoop()

func (d *DataChannel) onMessage(msg DataChannelMessage) {
    handler := d.onMessageHandler
    if handler == nil { return }   // ← 没有处理器时"静默丢弃"
    handler(msg)
}

// core/nethernet/listener.go：处理器要等"两个通道都打开"之后才注册
case "ReliableDataChannel":  conn.reliable = channel
case "UnreliableDataChannel": conn.unreliable = channel
if conn.reliable != nil && conn.unreliable != nil { close(opened) }
// ……返回后 handleConn 才调用 conn.handleTransports() 注册 OnMessage
```

网易客户端把 **DCEP 握手和第一个游戏包在同一毫秒发出来**，于是：

| 时刻 | 事件 |
|---|---|
| T+0.000s | 通道 1 打开 → `readLoop` 启动 |
| T+0.001s | 客户端发来 DCEP OPEN + `RequestNetworkSettings` |
| T+0.001s | OPEN 被 `Accept()` 消费；**`RequestNetworkSettings` 进入队列，但处理器还是 nil → 丢弃** |
| T+0.014s | 通道 3 打开 → 两个通道齐了 → 才注册 `OnMessage`（已经晚了） |

结果：Geyser 永远收不到 RequestNetworkSettings，客户端干等 90 秒超时。

### 修复

`core/nethernet/conn.go` 增加 `bindChannelHandlers()`（幂等），并在 **通道打开的瞬间**（listener 的 `OnDataChannelOpened`、dialer 的 `NewDataChannel` 之后）就绑定收发处理器：

```go
conn.sctp.OnDataChannelOpened(func(channel *webrtc.DataChannel) {
    switch channel.Label() { ... }
    conn.bindChannelHandlers(channel)   // ← 关键：不要等两个通道都齐
    if conn.reliable != nil && conn.unreliable != nil { close(opened) }
})
```

---

## 根因 ②：Java 握手 hostname 为空 → Velocity **静默**掐断

### 证据

在 Geyser 扩展里挂一个 `SessionListener` 逐包记录 Geyser↔Velocity（`-DGeyserNetease.Sniff=true`）：

```
诊断: forwardHostname=true joinAddress="" remoteServer=127.0.0.1:25565 auth=OFFLINE
      clientData.ServerAddress=":0"
出→ ClientIntentionPacket(protocolVersion=776, hostname=, port=25565, intent=LOGIN)   ← hostname 空！
出→ ServerboundHelloPacket(username=..., profileId=...)
== 已断开: reason=... key=disconnect.endOfStream, cause=null                        ← 一个字节都没回
```

`数据流终止` 不是 Velocity 发的：Velocity 4.0.0 全库**没有** `endOfStream` 这个字符串；它是 MCProtocolLib 在 `NetworkSession.channelInactive` 里写死的原版提示（`Component.translatable("disconnect.endOfStream")`）——只代表「通道被对端关闭」。

### 根因链

1. 网易局域网客户端的客户端数据里写着 `"ServerAddress":":0"`。
2. Geyser 的 `GeyserSession.joinAddress()`：

   ```java
   String combined = clientData.getServerAddress();   // ":0"
   int index = combined.lastIndexOf(":");
   return combined.substring(0, index);               // → ""
   ```

3. Geyser 配置 `forward-hostname: true` 时，`GeyserSessionAdapter.packetSending` 把 java 握手的 hostname 覆盖成 `joinAddress()` = **空字符串**；官方国际版客户端这里带的是玩家真实填写的地址，所以非空。
4. Velocity 收到空 hostname 后在登录阶段**静默关闭连接**：

   ```java
   // MinecraftConnection.exceptionCaught
   boolean frontlineHandler = activeSessionHandler instanceof InitialLoginSessionHandler
           || activeSessionHandler instanceof HandshakeSessionHandler
           || activeSessionHandler instanceof StatusSessionHandler;
   boolean willLog = !isQuietDecoderException && !frontlineHandler;   // 登录阶段恒为 false
   ...
   ctx.close();     // 不打任何日志
   ```

   同时解码失败抛的是 `DECODE_FAILED = new QuietRuntimeException(...)`、`handleUnknown`/`assertState` 也是直接 `close(true)` —— 于是「Velocity 日志一片空白」。

### 修复

扩展在设置客户端数据时把 `ServerAddress` 修正成真实地址（并额外做了一次握手 hostname 兜底改写）：

```java
private static final String FORCED_SERVER_ADDRESS =
    System.getProperty("GeyserNetease.ServerAddress", "example.com:49780");

// 若 clientData.ServerAddress 为空或以 ":" 开头 → 改写为 FORCED_SERVER_ADDRESS
patchServerAddress(session.getClientData());
```

修复后的同一份嗅探日志：

```
出→ ClientIntentionPacket(protocolVersion=776, hostname=example.com, port=25565, intent=LOGIN)
出→ ServerboundHelloPacket(username=ExamplePlayer, ...)
出→ ServerboundLoginAcknowledgedPacket
入← ClientboundLoginFinishedPacket(profile=GameProfile{id=00000000-..., name=...})
入← ClientboundLoginPacket
入← ClientboundLevelChunkWithLightPacket × 377      ← 世界数据正常下发，玩家进服
```

### 被排除的假设（附验证方式）

| 假设 | 如何排除 |
|---|---|
| 中文用户名导致 Velocity 拒绝 | 把 java 侧登录名改成纯 ASCII（`ExamplePlayer`）后**现象完全一样** → 与名字无关 |
| Velocity 装了会拦人的插件 | 服务器 `plugins/` 只有 Geyser |
| 目标服务器/后端配置错 | 同配置下官方国际版客户端进服正常（Velocity 有完整 `has connected` 日志） |
| 房间被回收 / 中转断开 | 同一时刻房间可查询、中转连接正常（网关日志有 `房间 xxx 存活`） |

---

## 排查手法（可复用）

1. **先分级定位**：客户端行为 → 网关日志 → `relay.log` → 目标服日志（Geyser/Velocity/后端），确认失败发生在哪一跳。
2. **网关侧留全量 hex**：`relay.log` 记录每个转发的原始字节；`go run ./cmd/diag/relaydecode relay.log` 直接解成包列表（自动剥 `0xFE`、解压、按帧解析）。
3. **Java 侧单独验证**：`go run ./cmd/diag/javaprobe -addr <java地址> -mode login -name X` 可以直接看到 Java 服务端回的是 `EncryptionRequest` / `LoginSuccess` / `Disconnect(原因)` / 直接 EOF。
4. **在目标服侧逐包取证**：Geyser 扩展的 `SessionListener`（`geyser-netease-java.log`）能记录 Geyser 发往代理的每一个包与代理回包——**「一方完全没回包」和「回了一个踢出包」是完全不同的结论**。
5. **读源码而不是猜**：`数据流终止` 这类文案先去源码里搜（Velocity 没有 → 说明是客户端库合成的）；Velocity 静默关闭的具体条件在 `MinecraftConnection.exceptionCaught` 里写得非常明确。
6. **A/B 对照**：同一套服务端，官方国际版客户端能进、网易客户端不能进 → 差异一定在两者不同的输入上（最终正是握手 hostname）。

## 复现与回归

```bash
# ① 首包丢失（网关侧）
#   期望：客户端连接后立刻出现 [转发] 玩家→服务器 7 字节: 06c1010000035c
type relay.log

# ② hostname 为空（目标服侧）
#   打开嗅探后进服，期望不再出现 hostname= 为空的 ClientIntentionPacket
#   -DGeyserNetease.Sniff=true   → 日志 geyser-netease-java.log
```
