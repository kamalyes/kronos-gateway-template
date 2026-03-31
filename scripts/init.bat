@echo off
setlocal enabledelayedexpansion
chcp 65001 >nul

rem 一键打通：解析协议中心依赖 -> go mod tidy -> build -> test
rem
rem go.mod 里协议中心的版本是占位符，直接 go mod tidy 解析不到：
rem 协议契约不在本仓库，而在协议中心（kronos-share-proto-template 生成的仓库）。
rem 协议中心还不存在时脚本会说清出路，而不是抛一串 Go 的 Repository not found。

set "PROTO_MODULE=<% .ShareProtoModule %>"
set "PROTO_VERSION=<% .ShareProtoVersion %>"
set "SKIP_TEST=false"
set "SKIP_PROTO=false"

:parse_args
if "%~1"=="" goto after_parse_args
if /i "%~1"=="--skip-test" set "SKIP_TEST=true" && shift && goto parse_args
if /i "%~1"=="--skip-proto" set "SKIP_PROTO=true" && shift && goto parse_args
if /i "%~1"=="--help" goto show_help
if /i "%~1"=="-h" goto show_help
echo ❌ 未知选项: %~1
exit /b 1

:show_help
echo 用法: scripts\init.bat [--skip-proto] [--skip-test]
echo   --skip-proto  跳过协议中心依赖解析
echo   --skip-test   跳过 go test 自检
exit /b 0

:after_parse_args

set "SCRIPT_DIR=%~dp0"
for %%i in ("%SCRIPT_DIR%..") do set "PROJECT_ROOT=%%~fi"
cd /d "%PROJECT_ROOT%"

echo ========================================
echo 🚀 初始化 <% .ServiceName %>
echo ========================================
echo.

where go >nul 2>&1
if errorlevel 1 (
    echo ❌ Go 未安装，请先安装 Go <% .GoVersion %> 或更高版本
    exit /b 1
)
for /f "tokens=3" %%v in ('go version') do echo ✅ Go: %%v
echo.

if "%SKIP_PROTO%"=="true" (
    echo ⚠️  [1/4] 按 --skip-proto 跳过协议中心依赖解析
) else (
    findstr /r /c:"^replace[ 	]*%PROTO_MODULE%" go.mod >nul 2>&1
    if not errorlevel 1 (
        echo ⚠️  [1/4] go.mod 里已打开本地 replace，跳过远端版本解析
    ) else (
        echo ℹ️  [1/4] 解析协议中心依赖 %PROTO_MODULE%@%PROTO_VERSION%
        go get %PROTO_MODULE%@%PROTO_VERSION%
        if errorlevel 1 (
            echo.
            echo ❌ 解析 %PROTO_MODULE%@%PROTO_VERSION% 失败
            echo.
            echo   本服务的 proto 契约来自协议中心，不在本仓库。通常是下面两种情况：
            echo.
            echo   一、协议中心仓库还没创建
            echo       用 kronos-share-proto-template 生成并推上去，再回来重跑：
            echo         kopy new https://github.com/kamalyes/kronos-share-proto-template -o ..\<% .ObjectPrefix %>-share-proto
            echo.
            echo   二、想先本地联调，暂不推远端
            echo       打开 go.mod 末尾那条 replace（去掉行首的 //），指向同级目录的协议仓库，
            echo       然后重跑：scripts\init.bat
            echo.
            echo   只想先看工程骨架：scripts\init.bat --skip-proto
            exit /b 1
        )
        echo ✅ 协议中心依赖已写入 go.mod
    )
)
echo.

echo ℹ️  [2/4] go mod tidy
go mod tidy
if errorlevel 1 (
    echo ❌ go mod tidy 失败
    exit /b 1
)
echo ✅ 依赖整理完成
echo.

echo ℹ️  [3/4] go build ./...
go build ./...
if errorlevel 1 (
    echo ❌ 编译失败
    exit /b 1
)
echo ✅ 编译通过
echo.

if "%SKIP_TEST%"=="true" (
    echo ⚠️  [4/4] 按 --skip-test 跳过测试
) else (
    echo ℹ️  [4/4] go test ./...
    go test ./...
    if errorlevel 1 (
        echo ❌ 测试失败
        exit /b 1
    )
    echo ✅ 测试通过
)

echo.
echo ========================================
echo ✅ 初始化完成
echo ========================================
echo.
echo 下一步：
echo   go run .                               启动服务（读 resources\<% .ProjectName %>-dev.yaml）
echo   git add -A ^&^& git commit -m "chore: bootstrap <% .ModuleName %>"
echo.
echo 接入 proto 定义的 service：在 bootstrap\services.go 的两个 TODO 处注册 gRPC 与 HTTP handler。
echo 参数校验不需要自己写：go-rpc-gateway 已把基于 go-argus 的 struct tag 校验拦截器挂在链上。
exit /b 0
