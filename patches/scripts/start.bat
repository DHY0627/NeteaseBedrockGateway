@echo off
chcp 65001 >nul
setlocal

rem ============================================================
rem  Velocity + Geyser + GeyserNetease launcher (Windows)
rem  Usage: double-click this file, or run start.bat in a terminal
rem  Extra args are passed through to Java, e.g.  start.bat -Xmx1G
rem ============================================================

cd /d "%~dp0"

rem ---- 1. locate java ----
set "JAVA=java"
where java >nul 2>nul
if errorlevel 1 (
    echo [错误] 找不到 java 命令。
    echo        请安装 JDK 17 或更高版本，并把它的 bin 目录加入 PATH。
    echo        下载： https://adoptium.net/temurin/releases/?version=17
    pause
    exit /b 1
)

rem ---- 2. check required files ----
if not exist "velocity.jar" (
    echo [错误] 当前目录下找不到 velocity.jar
    echo        请把你的 Velocity 主程序放在本目录，并改名为 velocity.jar
    echo        当前目录：%CD%
    pause
    exit /b 1
)
if not exist "plugins\Geyser-Velocity.jar" (
    echo [警告] 找不到 plugins\Geyser-Velocity.jar
    echo        网易版客户端将无法连接，请把 Geyser 的 Velocity 版插件放到该位置。
    echo.
)
if not exist "plugins\Geyser-Velocity\extensions\GeyserNeteaseExtension.jar" (
    echo [警告] 找不到 plugins\Geyser-Velocity\extensions\GeyserNeteaseExtension.jar
    echo        没有这个扩展，网易版客户端会卡在加载界面直到超时。
    echo.
)

rem ---- 3. start ----
rem   -Xms256M -Xmx512M  heap size, adjust as needed
echo ============================================================
echo  正在启动 Velocity ...
echo  目录：%CD%
echo ============================================================
echo.

java -Xms256M -Xmx512M %* -jar velocity.jar

echo.
echo ============================================================
echo  Velocity 已退出（退出码 %ERRORLEVEL%）
echo ============================================================
pause
endlocal
