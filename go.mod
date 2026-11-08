module github.com/<% .OrgName %>/<% .ProjectName %>

go <% .GoVersion %>

require (
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.30.0
	github.com/kamalyes/go-rpc-gateway v1.10.2-0.20261108113725-d288a36ccd0b
	github.com/kamalyes/go-toolbox v0.19.4
)

// 网关的依赖刻意只有这些：没有 gorm、没有 cachex、没有 sqlbuilder
//
// 它不碰任何存储，也不含业务逻辑——下游服务的 HTTP 路由由
// AutoRegisterWithHealthCheck 通过 gRPC reflection 自动建出来，
// 所以连下游的 pb 包都不用 import（除了鉴权契约）。
//
// 这也是判断「某段代码是不是该放进网关」的标准：如果它需要新的依赖，
// 大概就不该在这里。
//
// [EN] Deliberately minimal: no gorm, no cachex, no sqlbuilder. The gateway touches no storage
// and holds no business logic, and downstream HTTP routes are built automatically through gRPC
// reflection, so not even their pb packages are imported apart from the authorization contract.
// That is also the test for whether code belongs here: if it needs a new dependency, it
// probably does not.

// 这里故意不写协议中心的 require：由 scripts/init.sh 执行 go get 时自动加上
// [EN] The contract-hub require is deliberately absent and added by scripts/init.sh
