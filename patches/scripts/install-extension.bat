@echo off
chcp 65001 >nul
setlocal

rem ============================================================
rem  Install GeyserNetease extension (Windows)
rem  Usage: drag your Velocity folder onto this file,
rem         or double-click it and type the path when asked.
rem  What it does: copies the Geyser plugin and the extension to
rem         the correct places, handling the "extension folder
rem         must match the plugin jar name" rule automatically.
rem ============================================================

set "SRC=%~dp0"
set "DEST=%~1"

if "%DEST%"=="" (
    echo.
    echo 请输入你的 Velocity 目录完整路径（就是有 velocity.jar 的那个目录）：
    set /p DEST=
)

if "%DEST%"=="" (
    echo [错误] 没有提供路径。
    pause
    exit /b 1
)

rem strip quotes that come from drag-and-drop
set DEST=%DEST:"=%

if not exist "%DEST%\velocity.jar" (
    echo [错误] 在 "%DEST%" 里找不到 velocity.jar
    echo        请确认这是 Velocity 的根目录。
    pause
    exit /b 1
)

echo.
echo  源目录：%SRC%
echo  目标  ：%DEST%
echo.

rem ---- 1. Geyser plugin ----
if not exist "%DEST%\plugins" mkdir "%DEST%\plugins"
echo [1/2] 拷贝 Geyser 插件 ...
copy /Y "%SRC%geyser\Geyser-Velocity-2.10.1-b1174.jar" "%DEST%\plugins\Geyser-Velocity.jar" >nul
if errorlevel 1 (
    echo       [失败] 拷贝失败，可能是 Velocity 正在运行 —— 请先停掉再试。
    pause
    exit /b 1
)
echo       完成：plugins\Geyser-Velocity.jar

rem ---- 2. extension (folder name must match the plugin jar name) ----
echo [2/2] 安装 GeyserNetease 扩展 ...
set "EXTDIR=%DEST%\plugins\Geyser-Velocity\extensions"
if not exist "%EXTDIR%" mkdir "%EXTDIR%"
copy /Y "%SRC%extension\GeyserNeteaseExtension.jar" "%EXTDIR%\GeyserNeteaseExtension.jar" >nul
if errorlevel 1 (
    echo       [失败] 拷贝失败，可能是 Velocity 正在运行 —— 请先停掉再试。
    pause
    exit /b 1
)
echo       完成：plugins\Geyser-Velocity\extensions\GeyserNeteaseExtension.jar

echo.
echo ============================================================
echo  安装完成。
echo.
echo  注意：plugins\Geyser-Velocity\ 这个目录名必须和插件
echo        jar 的文件名对应。如果你把插件改成了别的名字，
echo        扩展目录也要同步改名，否则扩展不会被加载。
echo.
echo  接下来：
echo    1. 用 start.bat 启动 Velocity
echo    2. 在控制台确认出现「已启用扩展 GeyserNetEase」
echo    3. 改 velocity.toml 的 login-ratelimit 为 0
echo ============================================================
pause
endlocal
