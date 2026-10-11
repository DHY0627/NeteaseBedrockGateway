#!/bin/bash
# ============================================================
#  安装 GeyserNetease 扩展（Linux / macOS）
#  用法：
#     ./install-extension.sh /path/to/velocity
#  作用：把本包里的 Geyser 插件与扩展拷到正确位置，
#        并自动处理「扩展目录名必须与插件 jar 同名」的坑。
#  首次使用先赋可执行权限： chmod +x install-extension.sh
# ============================================================

set -u

SRC="$(cd "$(dirname "$0")" && pwd)"
DEST="${1:-}"

if [ -z "$DEST" ]; then
    echo
    read -r -p "请输入你的 Velocity 目录完整路径（就是有 velocity.jar 的那个目录）： " DEST
fi

if [ -z "$DEST" ]; then
    echo "[错误] 没有提供路径。"
    exit 1
fi

# 展开 ~ 与相对路径
DEST="$(cd "$DEST" 2>/dev/null && pwd)" || {
    echo "[错误] 目录不存在：$1"
    exit 1
}

if [ ! -f "$DEST/velocity.jar" ]; then
    echo "[错误] 在 $DEST 里找不到 velocity.jar"
    echo "       请确认这是 Velocity 的根目录。"
    exit 1
fi

echo
echo " 源目录：$SRC"
echo " 目标  ：$DEST"
echo

# ---- 1. Geyser 插件 ----
mkdir -p "$DEST/plugins"
echo "[1/2] 拷贝 Geyser 插件 ..."
if ! cp -f "$SRC/geyser/Geyser-Velocity-2.10.1-b1174.jar" "$DEST/plugins/Geyser-Velocity.jar"; then
    echo "      [失败] 拷贝失败，可能是 Velocity 正在运行 —— 请先停掉再试。"
    exit 1
fi
echo "      完成：plugins/Geyser-Velocity.jar"

# ---- 2. 扩展（目录名必须与插件 jar 同名）----
EXTDIR="$DEST/plugins/Geyser-Velocity/extensions"
mkdir -p "$EXTDIR"
echo "[2/2] 安装 GeyserNetease 扩展 ..."
if ! cp -f "$SRC/extension/GeyserNeteaseExtension.jar" "$EXTDIR/GeyserNeteaseExtension.jar"; then
    echo "      [失败] 拷贝失败，可能是 Velocity 正在运行 —— 请先停掉再试。"
    exit 1
fi
echo "      完成：plugins/Geyser-Velocity/extensions/GeyserNeteaseExtension.jar"

cat <<'EOF'

============================================================
 安装完成。

 注意：plugins/Geyser-Velocity/ 这个目录名必须和插件
       jar 的文件名对应。如果你把插件改成了别的名字，
       扩展目录也要同步改名，否则扩展不会被加载。

 接下来：
   1. 用 start.sh 启动 Velocity
   2. 在控制台确认出现「已启用扩展 GeyserNetEase」
   3. 改 velocity.toml 的 login-ratelimit 为 0
============================================================
EOF
