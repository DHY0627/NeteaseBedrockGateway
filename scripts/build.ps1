# 交叉编译 NeteaseBedrockGateway（Windows / Linux / macOS）
#
# 用法：
#   .\scripts\build.ps1                              # 默认编全部 9 个发布目标
#   .\scripts\build.ps1 -Targets windows/amd64       # 只编一个目标
#   .\scripts\build.ps1 -Targets linux/amd64,linux/armv7
#   .\scripts\build.ps1 -WithDiag                    # 顺便编常用诊断工具
#
# 目标写法：<os>/<arch>，32 位 ARM 用 armv7 表示（GOARM=7）
param(
    [string]$OutDir = "dist",
    [string[]]$Targets = @(
        "windows/386", "windows/amd64", "windows/arm64",
        "linux/386",   "linux/amd64",   "linux/armv7", "linux/arm64",
        "darwin/amd64", "darwin/arm64"
    ),
    [switch]$WithDiag
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $root
New-Item -ItemType Directory -Force -Path $OutDir | Out-Null

Write-Host "==> 检查 submodule（依赖在 third_party/ 下）" -ForegroundColor Cyan
$missing = @(git submodule status | Where-Object { $_.StartsWith("-") } | ForEach-Object { ($_ -split ' ')[1] })
if ($missing.Count -gt 0) {
    Write-Host "以下 submodule 还没拉取：" -ForegroundColor Red
    foreach ($m in $missing) { Write-Host "    $m" -ForegroundColor Red }
    Write-Host "请先执行：git submodule update --init --recursive" -ForegroundColor Yellow
    exit 1
}
go list -m all > $null
if ($LASTEXITCODE -ne 0) {
    Write-Host "依赖解析失败：请确认 third_party/ 下三个 submodule 都已就位。" -ForegroundColor Red
    exit 1
}

foreach ($target in $Targets) {
    $parts = $target.Split("/")
    $goos = $parts[0]
    $goarch = $parts[1]
    $goarm = ""
    # 允许写 armv7 / armv6 这种带 GOARM 的写法
    if ($goarch -match '^armv(\d+)$') {
        $goarm = $Matches[1]
        $goarch = "arm"
    }
    $ext = if ($goos -eq "windows") { ".exe" } else { "" }
    $suffix = if ($goarm) { "$goarch" + "v" + $goarm } else { $goarch }
    $name = "NeteaseBedrockGateway-$goos-$suffix$ext"

    $env:GOOS = $goos
    $env:GOARCH = $goarch
    $env:CGO_ENABLED = "0"
    if ($goarm) { $env:GOARM = $goarm } else { Remove-Item Env:GOARM -ErrorAction SilentlyContinue }

    Write-Host "==> 编译 $target" -ForegroundColor Cyan
    go build -trimpath -ldflags "-s -w" -o (Join-Path $OutDir $name) ./cmd/gateway
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    Write-Host "    $OutDir/$name"
}

if ($WithDiag) {
    Write-Host "==> 编译诊断工具（本机平台）" -ForegroundColor Cyan
    Remove-Item Env:GOOS -ErrorAction SilentlyContinue
    Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
    Remove-Item Env:GOARM -ErrorAction SilentlyContinue
    $ext = if ($IsWindows -or $env:OS -eq "Windows_NT") { ".exe" } else { "" }
    foreach ($tool in @("relaydecode", "javaprobe", "logindump", "chaininfo", "pcapsum", "fecheck")) {
        go build -trimpath -ldflags "-s -w" -o (Join-Path $OutDir "$tool$ext") "./cmd/diag/$tool"
        Write-Host "    $OutDir/$tool$ext"
    }
}

Write-Host "完成，产物在 $OutDir/" -ForegroundColor Green
Get-ChildItem $OutDir | Select-Object Name, Length | Format-Table -AutoSize
