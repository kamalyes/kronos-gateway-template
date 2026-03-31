#!/usr/bin/env bash
# 一键打通：解析协议中心依赖 → go mod tidy → build → test
#
# 为什么需要这个脚本：go.mod 里协议中心的版本是占位符
# （v0.0.0-00010101000000-000000000000），直接 go mod tidy 解析不到。
# 协议契约不在本仓库，而在协议中心（kronos-share-proto-template 生成的仓库），
# 它的版本得先解析出来写进 go.mod。
#
# 协议中心还不存在时脚本会说清两条出路，而不是抛一串 Go 的 "Repository not found"。
#
# 幂等：重复执行只是重新解析并再 tidy 一遍，不会累积副作用。
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_ROOT"

PROTO_MODULE="<% .ShareProtoModule %>"
PROTO_VERSION="<% .ShareProtoVersion %>"

if [[ -t 1 ]]; then
    C_GREEN=$'\033[32m'; C_YELLOW=$'\033[33m'; C_RED=$'\033[31m'; C_CYAN=$'\033[36m'; C_OFF=$'\033[0m'
else
    C_GREEN=""; C_YELLOW=""; C_RED=""; C_CYAN=""; C_OFF=""
fi
log_info()    { echo "${C_CYAN}ℹ️  $1${C_OFF}"; }
log_success() { echo "${C_GREEN}✅ $1${C_OFF}"; }
log_warn()    { echo "${C_YELLOW}⚠️  $1${C_OFF}"; }
log_error()   { echo "${C_RED}❌ $1${C_OFF}" >&2; }

SKIP_TEST=0
SKIP_PROTO=0
for arg in "$@"; do
    case "$arg" in
        --skip-test)  SKIP_TEST=1 ;;
        --skip-proto) SKIP_PROTO=1 ;;
        --help|-h)
            echo "用法: bash scripts/init.sh [--skip-proto] [--skip-test]"
            echo "  --skip-proto  跳过协议中心依赖解析（已手工指定版本或已打开本地 replace 时用）"
            echo "  --skip-test   跳过 go test 自检"
            exit 0
            ;;
        *) log_error "未知选项: $arg"; exit 1 ;;
    esac
done

echo "========================================"
echo "🚀 初始化 <% .ServiceName %>"
echo "========================================"
echo ""

if ! command -v go >/dev/null 2>&1; then
    log_error "Go 未安装，请先安装 Go <% .GoVersion %> 或更高版本"
    exit 1
fi
log_success "Go: $(go version | awk '{print $3}')"
echo ""

# ── 1/4 协议中心依赖 ────────────────────────────────────────────────────────
# go.mod 里已打开本地 replace 时不必解析远端版本，占位符也不会被用到
if grep -qE '^replace[[:space:]]+<% .ShareProtoModule %>' go.mod 2>/dev/null; then
    log_warn "[1/4] go.mod 里已打开本地 replace，跳过远端版本解析"
elif [[ "$SKIP_PROTO" == 1 ]]; then
    log_warn "[1/4] 按 --skip-proto 跳过协议中心依赖解析"
else
    log_info "[1/4] 解析协议中心依赖 ${PROTO_MODULE}@${PROTO_VERSION}"
    # 先摘掉已有的 require 再 go get：go get 会先解析 go.mod 里现存的版本约束，
    # 若那里是个占位或写坏的版本，会直接报 invalid version 而拿不到 latest
    # 模块本就不在 require 时 droprequire 是无副作用的空操作
    #
    # [EN] Drop any existing requirement first: go get resolves the version already in
    # go.mod, so a placeholder or broken one makes it fail instead of fetching latest
    go mod edit -droprequire="${PROTO_MODULE}" 2>/dev/null || true
    if ! go get "${PROTO_MODULE}@${PROTO_VERSION}"; then
        echo ""
        log_error "解析 ${PROTO_MODULE}@${PROTO_VERSION} 失败"
        cat >&2 <<HINT

  本服务的 proto 契约来自协议中心，不在本仓库。上面的失败通常是下面两种情况之一：

  一、协议中心仓库还没创建
      用 kronos-share-proto-template 生成并推上去，再回来重跑本脚本：
        kopy new https://github.com/kamalyes/kronos-share-proto-template -o ../<% .ObjectPrefix %>-share-proto

  二、想先本地联调，暂不推远端
      打开 go.mod 末尾那条 replace（去掉行首的 //），指向同级目录的协议仓库，
      然后重跑：bash scripts/init.sh

  只想先看工程骨架、暂时不接协议中心：bash scripts/init.sh --skip-proto
  （此时 models/base.go 会编译不过，因为它用了协议中心的 pb/common）
HINT
        exit 1
    fi
    log_success "协议中心依赖已写入 go.mod"
fi
echo ""

# ── 2/4 依赖整理 ────────────────────────────────────────────────────────────
log_info "[2/4] go mod tidy"
go mod tidy
log_success "依赖整理完成"
echo ""

# ── 3/4 编译 ────────────────────────────────────────────────────────────────
log_info "[3/4] go build ./..."
go build ./...
log_success "编译通过"
echo ""

# ── 4/4 自检 ────────────────────────────────────────────────────────────────
if [[ "$SKIP_TEST" == 0 ]]; then
    log_info "[4/4] go test ./..."
    go test ./...
    log_success "测试通过"
else
    log_warn "[4/4] 按 --skip-test 跳过测试"
fi

echo ""
echo "========================================"
log_success "初始化完成"
echo "========================================"
echo ""
echo "下一步："
echo "  go run .                               # 启动服务（APP_ENV 未设置时读 resources/<% .ProjectName %>-dev.yaml）"
echo "  git add -A && git commit -m \"chore: bootstrap <% .ModuleName %>\""
echo ""
echo "接入 proto 定义的 service：在 bootstrap/services.go 的两个 TODO 处注册 gRPC 与 HTTP handler。"
echo "参数校验不需要自己写：go-rpc-gateway 已把基于 go-argus 的 struct tag 校验拦截器挂在链上，"
echo "proto 里 @inject_tag: validate:\"...\" 注入的规则自动生效。"
