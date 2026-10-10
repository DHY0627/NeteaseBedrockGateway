# Web 控制台（前端演示版）

模仿 [DDNS-Go](https://github.com/jeessy2/ddns-go) 的浅色卡片风格，纯静态页面，**零依赖、零构建**。

> ⚠️ 当前**只是前端演示**：所有「启动/停止/日志/保存」都是前端 mock，
> 数据存在浏览器 localStorage（键 `nbg.demo.state.v1`），未接入网关。
> 清掉站点数据即可恢复默认演示数据。

## 立刻预览

直接双击打开（`file://` 也能跑，没有 fetch/XHR）：

```
login.html          登录页
index.html          管理页（未登录会自动跳回登录页）
```

想用本地 HTTP 起也行：

```bash
cd cmd/gateway/webui && python3 -m http.server 8090   # 纯静态预览（没有后端，接口会 404）
```

## 文件

| 文件 | 作用 |
|---|---|
| `login.html` | 登录页。登录态用 `sessionStorage`，**关闭标签页即失效** |
| `index.html` | 管理页：账号管理 / 实例列表 / 全局设置 + 三个弹窗 |
| `assets/style.css` | 全部样式（主题色、卡片、表格、徽章、弹窗、日志查看器） |
| `assets/app.js` | 全部交互逻辑（含 mock 数据与渲染） |

## 页面结构

```
登录页
 └─ 用户名 / 密码 → 登录（一次性：关标签页退出）

管理页
 ├─ 账号管理        多账号增删改（4399 账号，供实例选用）
 ├─ 网关实例  [创建实例]
 │   └─ 每个实例一张卡片
 │       ├─ 状态徽章（运行中 / 已停止）、启动·停止、打开网关日志、编辑、删除
 │       ├─ 使用账号 · 房间号 · 服务器 · 在线玩家 · 房间密码 · 命令数
 │       └─ 命令设置：每行 = 触发条件 + 命令行（可停用/编辑/删除）＋「创建命令行」
 └─ 全局设置        用户名 / 密码（留空=不改） / 公网访问 / 取消·保存
```

弹窗三个：**创建·编辑实例**、**创建·编辑命令行**、**网关日志**。

## 命令行占位符

点击弹窗里的占位符即可插入到光标处，执行时替换为实际值：

| 占位符 | 含义 |
|---|---|
| `{&roomid}` | 当前房间号 |
| `{&timestamp}` | Unix 时间戳（秒） |
| `{&datetime}` | 本地时间，如 `2026-10-10 03:20:00` |
| `{&account}` | 实例使用的 4399 账号 |
| `{&instancename}` | 实例名称 |
| `{&server}` | 转发目标 `IP:端口` |
| `{&capacity}` | 房间最大人数 |
| `{&players}` | 当前在线玩家数 |
| `{&event}` | 触发事件（如 `room.changed`） |

触发条件：网关启动 / 网关停止 / 登录成功 / 房间创建成功 / 房间号变化 / 房间重建 /
玩家加入 / 玩家离开 / 存活检查失败。

---

## 接入后端时的接口约定（待实现）

前端目前不发任何请求；后端按下面这套实现即可，前端只需把 mock 换成 `fetch`。

| 方法 | 路径 | 说明 |
|---|---|---|
| `POST` | `/api/login` | `{username, password}` → 成功返回 `Set-Cookie: nbg_session=…`（**会话 Cookie，不设 Max-Age/Expires**，即浏览器关闭即失效，正好对应「一次性登录」） |
| `POST` | `/api/logout` | 注销并清 Cookie |
| `GET` | `/api/session` | 当前登录状态（401 表示未登录） |
| `GET` `POST` `PUT` `DELETE` | `/api/accounts[/{id}]` | 账号管理（`{note, account, password}`） |
| `GET` `POST` `PUT` `DELETE` | `/api/instances[/{id}]` | 实例管理（`{name, accountId, target, roomPassword, capacity}`） |
| `POST` | `/api/instances/{id}/start` · `/stop` | 启停实例 |
| `GET` | `/api/instances/{id}/status` | `{status, players, roomId, uptime, recreations}` |
| `GET` | `/api/instances/{id}/logs?tail=500` | 该实例日志（建议 SSE 或长轮询做实时） |
| `GET` `POST` `PUT` `DELETE` | `/api/instances/{id}/commands[/{cmdId}]` | 命令行（`{trigger, line, note, enabled}`） |
| `GET` `POST` | `/api/settings` | 全局设置（`{username, password?, publicAccess}`，密码为空表示不改） |

### 样式契约

前端渲染用的类名即数据形状的最小约定，后端返回的字段名建议与 mock 保持一致：

```js
account  = { id, note, account, password }                       // password 回显时可用 "******"
instance = { id, name, accountId, target, roomPassword, capacity,
             status: 'running' | 'stopped', roomId, players, commands: [command] }
command  = { id, trigger, line, note, enabled }
settings = { user, publicAccess }
```

---

## 启动参数（本次新增，待接入后端）

| 参数 | 说明 |
|---|---|
| `-port <0-65535>` | Web 控制台端口号（默认 8765，绑定 `0.0.0.0` 与 `[::]`） |
| `-install` | 注册为 systemd 服务（仅 Linux） |

> ⚠️ **命名冲突待确认**：现有网关里 `-p` 已经是「4399 密码」。
> 两者不能共用同一个短参数，需要二选一：
> 1. Web 端口改用别的短参数（例如 `-wp` / `-port` / `-web-port`），`-p` 继续表示密码；
> 2. 或 `-p` 改为端口，4399 密码改由账号配置文件 / `-password` 提供。
> 前端暂按「端口」展示，实现后端前请先定这一条。
