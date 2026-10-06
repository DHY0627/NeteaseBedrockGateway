# docs/captures —— 抓包与 hex 证据

这个目录存放排错期间留下的原始证据，**默认不入库**（见根目录 `.gitignore`：`*.pcap` / `*.hex` / `docs/captures/*`），
因为其中的 `login_*.hex`、`*.pcap` **包含真实网易账号的身份链/JWT/token**。

## 内容

| 文件 | 说明 |
|---|---|
| `cap.pcap` / `full.pcap` / `lan.pcap` / `trig.pcap` / `host.pcap` / `self.pcap` | 模拟器/本机抓包：网易本地联机（RakNet + NetherNet）与握手过程 |
| `login_*.hex` | 玩家 `Login` 包（解压后批次）样本：真实客户端、被改写版、伪造链版等 |
| `handshake*.hex` | Geyser `ServerToClientHandshake` 原始字节 |
| `msg*.hex` | TanLobby / NetherNet 单条消息样本 |
| `r26.hex` / `r63.hex` / `resp*.hex` / `realhs.hex` | 真实房主/服务器的响应样本（NetworkSettingsResponse、ResourcePackStack 等） |

## 使用

```bash
# 解码某段 hex（例如玩家 Login 批次）
go run ./cmd/diag/deflateprobe docs/captures/r63.hex

# 解析身份链结构
go run ./cmd/diag/chaininfo docs/captures/login_aminuosi.hex

# 概览 pcap 里的 UDP 流
go run ./cmd/diag/pcapsum docs/captures/cap.pcap
```

## 注意

- **不要**把这些文件提交到公开仓库，也不要在 issue/聊天里粘贴完整内容（等于泄露账号身份）。
- 需要分享时，请用 `cmd/diag/chaininfo` 之类工具输出**结构摘要**，并抹掉 `identity` / `Token` / `netease_uid` 等字段。
