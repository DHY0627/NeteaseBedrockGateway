# 启动 / 安装脚本

配合上级目录 `dist/` 里的预编译扩展使用。

| 脚本 | 平台 | 作用 |
|---|---|---|
| `start.bat` | Windows | 启动 Velocity（自动找 Java、校验 JDK 17+、检查文件是否就位） |
| `start.sh` | Linux / macOS | 同上 |
| `install-extension.bat` | Windows | 把 Geyser 插件 + 扩展装到正确位置 |
| `install-extension.sh` | Linux / macOS | 同上 |

## 用法

把脚本放到你的 Velocity 根目录（有 `velocity.jar` 的那一层），
并保证同级有 `geyser/` 与 `extension/` 两个目录（即上级 README 里的目录结构）。

```bat
:: Windows
install-extension.bat "D:\mc\velocity"     :: 也可以把 Velocity 目录直接拖到文件上
start.bat                                   :: 或 start.bat -Xmx1G
```

```bash
# Linux / macOS
chmod +x start.sh install-extension.sh
./install-extension.sh /home/you/velocity
./start.sh
```

两个脚本都会在动作前做检查，缺什么会明确说明缺在哪里。

## 修改脚本时的两个硬性要求

`start.bat` / `install-extension.bat` 若要自己改，注意：

1. **行尾必须是 CRLF**。LF 换行的 bat 会被 `cmd.exe` 错误解析，报
   `'xxx' is not recognized as an internal or external command`。
   仓库已通过 `.gitattributes` 对 `*.bat` 强制 CRLF。
2. **`rem` 注释里不要出现中文或全角符号**。`chcp 65001` 切换代码页后，
   注释中的非 ASCII 字节会被 cmd 错误切分，把注释后半段当命令执行。
   中文只放在 `echo` 的输出里（那里没问题）。

`start.sh` / `install-extension.sh` 保持纯 LF，不要转成 CRLF，
否则 Linux 上会报 `$'\r': command not found`。

## 实测记录

- 两个 `sh` 脚本在 Linux aarch64 上实跑通过（Velocity 3.5.1 正常启动）
- 两个 `bat` 脚本在 Windows `cmd` 下实跑通过，安装 + 启动后确认
  `NetEase Extension initialized — RakNet v8 clients supported.` 出现
- 含空格的路径、带引号的拖拽参数均正确处理