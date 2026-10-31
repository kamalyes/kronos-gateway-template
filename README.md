# kronos-gateway-service

**简体中文** | [English](./README.en.md)

网关：唯一讲 HTTP 的进程。把下游 gRPC 服务的 HTTP 接口聚合起来对外提供,并在每个请求前调鉴权服务判断是否放行。

**它没有业务代码。** 整个仓库只有 4 个 Go 文件,其中唯一的逻辑是鉴权中间件。

## 为什么这么短

下游服务的 HTTP 路由不是手写的。`AutoRegisterWithHealthCheck` 遍历配置里的 `grpc.clients`,连上每个下游,通过 **gRPC server reflection** 读取它们的服务定义与 `google.api.http` 注解,然后在**网关进程里**建出对应的 HTTP 路由。

所以:

- 加一个下游服务 = 在 `resources/` 的 `grpc.clients` 里加一条,**不改一行 Go 代码**,也不用 `import` 它的 pb 包
- 下游加一个接口 = 下游自己加 proto 注解,网关重启后自动有了

两个前提:

| 前提 | 不满足的现象 |
|---|---|
| 下游必须开 `enable-reflection` | 那个服务的接口**全部 404,但服务明明活着** |
| RPC 必须有 `google.api.http` 注解 | 那个 RPC 没有 HTTP 入口 |

第二条是特性不是缺陷:`AuthService.Authorities` 就刻意不加注解,于是它永远拿不到 HTTP 路由 —— 否则任何能打到网关的人都能直接调鉴权接口,而它的返回值里带着用户身份。

启动时会打一行发现结果摘要。**这行必须看** —— 下游名字拼错、reflection 没开、下游还没起来,这三种情况都不会让网关启动失败,只会让某些路由不存在。不看它,排查时会从「接口 404」一路猜到网络策略。

## 四个模板的分工

| 模板 | 角色 | gRPC | 自己的 HTTP | 业务逻辑 |
|---|---|---|---|---|
| `kronos-service-template` | 业务服务（有库有缓存） | ✅ | ✅ | ✅ |
| `kronos-key-service-template` | 基础设施（密钥与发号） | ✅ | ❌ | ✅ |
| `kronos-access-control-template` | 鉴权 | ✅ | ❌ | ✅ |
| **本模板** | HTTP 面 | ❌ | ✅ | ❌ |

请求流向:

```
浏览器 ──HTTP──▶ 网关 ──Authorities(path, method)──▶ 鉴权服务
                  │                                    │
                  │◀────── allowed + Payload ──────────┘
                  │
                  └──gRPC（Payload 注入 header）──▶ 业务服务 / key-service
```

## 鉴权中间件

`bootstrap/middleware.go`,网关唯一的业务逻辑。

1. 从连接池取鉴权服务的连接（不自己拨号 —— 池里的连接已经挂好了健康检查与链路透传拦截器）
2. 调 `Authorities(path, method)`,请求的 metadata 整体透传（token 在 `authorization` 头里,协议刻意没有 token 字段）
3. 拒绝 → 按 `token_valid` 决定 401 还是 403
4. 放行 → 把 `Payload` 注入 HTTP header 与 context,交给下一环

### 401 还是 403

| `token_valid` | HTTP | 含义 | 客户端该做什么 |
|---|---|---|---|
| `false` | 401 | 未认证（缺失/过期/伪造） | 重新登录 |
| `true` | 403 | 已认证但权限不足 | 重新登录也没用 |

混为一谈会让前端在权限不足时反复跳登录页,用户陷在登录循环里。

### 鉴权不可用时返回 500,不放行

放行等于在鉴权服务挂掉的几分钟里把所有接口变成公开接口。也不返回 401 —— 那会误导客户端去重新登录,而问题不在它的凭据上,只会在故障期间给登录接口再加一波流量。

### Payload 注入是这套架构的关键

**下游服务不重复验 token**,它们从 header 里读身份。所以这里注入什么,下游就信什么。

验一次就够。验两次意味着两处逻辑要永远一致,而它们迟早不一致 —— 不一致时「网关放行、下游拒绝」是好结果,「网关拒绝、下游放行」才是事故。

**前提:下游服务绝不能直接暴露给外网。** 谁能直连它们谁就能伪造任意用户。部署时靠网络策略保证,不是靠代码。

header 和 context 都要写:header 给下游（跨进程）,context 给本进程后续的中间件和日志。只写 header 的话,网关自己的访问日志里没有用户身份。

完整 Payload 走 Base64 —— 昵称等字段可能含非 ASCII 字符,而 gRPC metadata 的值只允许 ASCII,非 ASCII 会让整个请求在 metadata 校验阶段失败。

### 一个网关只服务一个域

`gatewayDomain` 在编译期固定（由 kopy 变量渲染）,不从配置读:配置可以热更新,而「这个网关是哪一侧的」是部署决策,不该能在运行时被改掉。

中间件会校验鉴权服务返回的域与之一致。**这一道比它看起来重要** —— 运维网关和租户网关通常共用同一个鉴权服务,不校验的话,一个租户用户的 token 拿到运维网关上去用,鉴权服务会认为它合法（token 确实有效）,于是租户用户越权进了运维接口。

要同时服务两侧,部署两个实例各配一份 `resources`,而不是在一个进程里按路径分流 —— 分流规则会被改,改错的后果是跨域越权,且现象是「某些接口突然能访问了」,不会报错。

## 一个最危险的配置错误

鉴权中间件必须在 `RebuildHTTPGateway` **之前**挂上。中间件是在重建时被织进 mux 的,之后再挂不会生效,而现象是**所有接口都免鉴权且一切正常** —— 没有任何报错。

`bootstrap/server.go` 里的顺序就是对的,改那个文件时注意别把两行调换。

## 快速开始

```bash
kopy new https://github.com/kamalyes/kronos-gateway-template -o ./kronos-gateway-service
```

网关本身没有外部依赖（不碰存储）,但它需要下游服务活着才有路由可建。完整跑通一次:

```bash
# 1. 鉴权服务（桩实现，无外部依赖）
cd kronos-access-control-service && go run .     # :9390

# 2. 密钥与发号服务（需要 Redis）
docker run -d -p 6379:6379 redis:7-alpine
cd kronos-key-service && go run .                # :9290

# 3. 业务服务（需要 MySQL + Redis）
cd kronos-my-service && go run .                 # :9190

# 4. 网关
cd kronos-gateway-service && go run .                # :8490
```

然后:

```bash
# 免鉴权路径
curl localhost:8490/health

# 业务接口不带 token → 401
curl -i localhost:8490/v1/products

# 带任意 token（桩鉴权一律放行）→ 请求被转发到业务服务
curl -H 'authorization: Bearer whatever' localhost:8490/v1/products

# 密钥接口：key-service 自己没有 HTTP 入口，这条路由只存在于网关
curl -X POST -H 'authorization: Bearer whatever' \
  -H 'content-type: application/json' \
  -d '{"scenario":"KEY_SCENARIO_LOGIN"}' \
  localhost:8490/v1/temp-keys/issue
```

最后那条是这套架构最值得看的一点:`key-service` 刻意不注册 HTTP handler,所以密钥接口的**唯一入口是网关**,必然经过鉴权中间件。即使有人把 key-service 的 HTTP 端口暴露出去,那上面也没有密钥接口可打。

## 服务名必须三处一致

`constants/service_name.go` 里的三个下游服务名,必须与下面三处完全一致:

1. `resources/kronos-gateway-service-dev.yaml` 里 `grpc.clients` 的 key
2. 对应服务自己的 `constants.ServiceName`
3. 这里

连接池按服务名索引。差一个字符的现象是网关每个请求都报「取不到鉴权服务连接」,而那个服务明明活着。

## 目录结构

```
kronos-gateway-service/
├── main.go
├── bootstrap/
│   ├── server.go       启动序列；AutoRegister 在挂中间件之前
│   ├── middleware.go   鉴权中间件（网关唯一的业务逻辑）
│   ├── domain.go       本网关服务的域，编译期固定
│   └── logger.go       日志上下文字段
├── constants/
│   └── service_name.go 本服务名 + 三个下游服务名
├── locales/            鉴权失败的本地化文案
└── resources/
    └── kronos-gateway-service-dev.yaml   grpc.clients 是「代理哪些服务」的唯一声明
```
