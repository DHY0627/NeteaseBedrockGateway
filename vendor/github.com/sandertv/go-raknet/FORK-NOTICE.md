# FORK-NOTICE — NeteaseBedrockGateway 专用 fork

本仓库是 [sandertv/go-raknet](https://github.com/sandertv/go-raknet) 的 fork，供
[NeteaseBedrockGateway](https://github.com/DHY0627/NeteaseBedrockGateway) 使用。

- 基线：上游 tag **`v1.15.1`**（提交 `703ec90`），分支名 `netease`
- 模块路径保持上游不变：`github.com/sandertv/go-raknet`
  （`go.mod` 里用 `replace github.com/sandertv/go-raknet => github.com/DHY0627/go-raknet <伪版本>` 引用本 fork）
- 许可：MIT，见 `LICENSE`，版权归原作者 sandertv 及贡献者；本 fork 未改动许可，也未新增条款

## 相对上游的改动（只有 1 个文件）

| 文件 | 改动 | 原因 |
| --- | --- | --- |
| `conn.go` | `protocolVersion` 由 `11` 改为 `8` | 网易（中国版）客户端的 RakNet 只接受协议版本 8，走 `NETEASE_RAKNET` 分支；发版本 11 时对端直接忽略、不回包，表现为「连上了但没有任何数据」 |

除 `conn.go` 外文件清单与上游 `v1.15.1` 完全一致（41 个文件），没有其他改动。

## 为什么必须 fork

`protocolVersion` 是 `conn.go` 里 `const` 块内的私有常量，硬编码为 11，无法从外部配置，
所以使用方只能 fork 或打补丁。它同时被 `Listener`（服务端）和 `Dial`（客户端）使用：

- 本 fork 只服务于「连网易 Geyser / BDS」的场景；
- 若拿它连普通（国际版）Bedrock 服务器，可能因协议版本 8 被对端拒绝。

## 建议上游 PR

把 `protocolVersion` 改成可配置（例如 `DialConfig{ProtocolVersion: ...}`，或导出为变量并保留
默认值 11），这个 fork 就可以废弃。欢迎上游采纳。

## 与上游 `master` 的关系

上游 `master` 在 `v1.15.1` 之后另有 6 个提交（`open_connection_request_1` 缓存的互斥锁保护、
大包断连修复、`SystemUptime` 跟踪、CI 调整等）。本 fork **有意停在 `v1.15.1`**：
NeteaseBedrockGateway 的端到端验证就是基于这一版做的，升级到 `master` 需要重新验证。
