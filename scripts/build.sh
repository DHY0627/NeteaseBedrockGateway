#!/usr/bin/env bash
# 交叉编译 NeteaseBedrockGateway（Linux / Windows / macOS）
#
# 用法：
#   ./scripts/build.sh                                   # 默认编 linux/amd64 + linux/arm64 + windows/amd64
#   ./scripts/build.sh linux/amd64                       # 只编一个目标
#   WITH_DIAG=1 ./scripts/build.sh                       # 顺便编常用诊断工具
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

OUT_DIR="${OUT_DIR:-dist}"
TARGETS=("${@:-linux/amd64 linux/arm64 windows/amd64}")
mkdir -p "$OUT_DIR"

echo "==> 检查依赖（go.mod 的本地 replace 必须就位）"
if ! go list -m all >/dev/null 2>&1; then
  echo "依赖缺失：请先按 README「依赖准备」把 nemc-tan-lobby-solver / go-raknet-netease / g79client 放到同级目录。" >&2
  exit 1
fi

for target in "${TARGETS[@]}"; do
  goos="${target%%/*}"
  goarch="${target##*/}"
  ext=""
  [ "$goos" = "windows" ] && ext=".exe"
  name="NeteaseBedrockGateway-${goos}-${goarch}${ext}"

  echo "==> 编译 ${target}"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
    go build -trimpath -ldflags "-s -w" -o "$OUT_DIR/$name" ./cmd/gateway
  echo "    $OUT_DIR/$name"
done

if [ "${WITH_DIAG:-0}" = "1" ]; then
  echo "==> 编译诊断工具（本机平台）"
  for tool in relaydecode javaprobe logindump chaininfo pcapsum fecheck; do
    go build -trimpath -ldflags "-s -w" -o "$OUT_DIR/$tool" "./cmd/diag/$tool"
    echo "    $OUT_DIR/$tool"
  done
fi

echo "完成，产物在 $OUT_DIR/"
ls -lh "$OUT_DIR"
