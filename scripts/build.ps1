# 交叉编译 NeteaseBedrockGateway（Windows / Linux / macOS）
#
# 用法：
#   .\scripts\build.ps1                          # 默认编 windows/amd64 + linux/amd64 + linux/arm64
#   .\scripts\build.ps1 -Targets windows/amd64   # 只编一个目标
#   .\scripts\build.ps1 -WithDiag                # 顺便编常用诊断工具
param(
    [string]$OutDir = "dist",
    [string[]]$Targets = @("windows/amd64", "linux/amd64", "linux/arm64"),
    [switch]$WithDiag
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $root
New-Item -ItemType Directory -Force -Path $OutDir | Out-Null

Write-Host "==> 检查依赖（go.mod 的本地 replace 必须就位）" -ForegroundColor Cyan
go list -m all > $null
if ($LASTEXITCODE -ne 0) {
    Write-Host "依赖缺失：请先按 README「依赖准备」把 nemc-tan-lobby-solver / go-raknet-netease / g79client 放到同级目录。" -ForegroundColor Red
    exit 1
}

foreach ($target in $Targets) {
    $parts = $target.Split("/")
    $goos = $parts[0]
    $goarch = $parts[1]
    $ext = if ($goos -eq "windows") { ".exe" } else { "" }
    $name = "NeteaseBedrockGateway-$goos-$goarch$ext"

    $env:GOOS = $goos
    $env:GOARCH = $goarch
    $env:CGO_ENABLED = "0"

    Write-Host "==> 编译 $target" -ForegroundColor Cyan
    go build -trimpath -ldflags "-s -w" -o (Join-Path $OutDir $name) ./cmd/gateway
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    Write-Host "    $OutDir/$name"
}

if ($WithDiag) {
    Write-Host "==> 编译诊断工具（本机平台）" -ForegroundColor Cyan
    Remove-Item Env:GOOS -ErrorAction SilentlyContinue
    Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
    $ext = if ($IsWindows -or $env:OS -eq "Windows_NT") { ".exe" } else { "" }
    foreach ($tool in @("relaydecode", "javaprobe", "logindump", "chaininfo", "pcapsum", "fecheck")) {
        go build -trimpath -ldflags "-s -w" -o (Join-Path $OutDir "$tool$ext") "./cmd/diag/$tool"
        Write-Host "    $OutDir/$tool$ext"
    }
}

Write-Host "完成，产物在 $OutDir/" -ForegroundColor Green
Get-ChildItem $OutDir | Select-Object Name, Length | Format-Table -AutoSize
