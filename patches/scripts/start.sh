#!/bin/bash
# ============================================================
#  Velocity + Geyser + GeyserNetease 启动脚本（Linux / macOS）
#  用法：
#     ./start.sh              # 前台启动
#     ./start.sh -Xmx1G       # 额外参数原样传给 Java
#  首次使用先赋可执行权限： chmod +x start.sh
# ============================================================

set -u

cd "$(dirname "$0")" || exit 1

# ---- 1. 找 Java ----
if ! command -v java >/dev/null 2>&1; then
    echo "[错误] 找不到 java 命令。"
    echo "       请安装 JDK 17 或更高版本。"
    echo "       Debian/Ubuntu:  sudo apt install openjdk-17-jre-headless"
    echo "       CentOS/RHEL:    sudo dnf install java-17-openjdk-headless"
    echo "       macOS:          brew install openjdk@17"
    exit 1
fi

# 检查 Java 主版本 >= 17（Velocity 与 Geyser 的硬要求）
JVER="$(java -version 2>&1 | head -n 1 | sed -E 's/.*version "([0-9]+).*/\1/')"
if [ -n "$JVER" ] && [ "$JVER" -lt 17 ] 2>/dev/null; then
    echo "[错误] 当前 Java 主版本是 $JVER，Velocity 需要 17 或更高。"
    java -version
    exit 1
fi

# ---- 2. 检查必要文件 ----
if [ ! -f "velocity.jar" ]; then
    echo "[错误] 当前目录下找不到 velocity.jar"
    echo "       请把你的 Velocity 主程序放在本目录，并改名为 velocity.jar"
    echo "       当前目录：$(pwd)"
    exit 1
fi
if [ ! -f "plugins/Geyser-Velocity.jar" ]; then
    echo "[警告] 找不到 plugins/Geyser-Velocity.jar"
    echo "       网易版客户端将无法连接，请把 Geyser 的 Velocity 版插件放到该位置。"
    echo
fi
if [ ! -f "plugins/Geyser-Velocity/extensions/GeyserNeteaseExtension.jar" ]; then
    echo "[警告] 找不到 plugins/Geyser-Velocity/extensions/GeyserNeteaseExtension.jar"
    echo "       没有这个扩展，网易版客户端会卡在加载界面直到超时。"
    echo
fi

# ---- 3. 启动 ----
#   -Xms256M -Xmx512M  堆内存，按需调整
echo "============================================================"
echo " 正在启动 Velocity ..."
echo " 目录：$(pwd)"
echo "============================================================"
echo

exec java -Xms256M -Xmx512M "$@" -jar velocity.jar
