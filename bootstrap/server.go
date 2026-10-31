/*
 * @Author: kamalyes example@qq.com
 * @Date: 2026-10-31 00:00:00
 * @LastEditors: kamalyes example@qq.com
 * @LastEditTime: 2026-10-31 00:00:00
 * @FilePath: \kronos-gateway-service\bootstrap\server.go
 * @Description: 网关启动引导
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package bootstrap

import (
	"fmt"

	"github.com/kamalyes/kronos-gateway-service/constants"
	goconfig "github.com/kamalyes/go-config"
	gateway "github.com/kamalyes/go-rpc-gateway"
	grpcpool "github.com/kamalyes/go-rpc-gateway/cpool/grpc"
	gwglobal "github.com/kamalyes/go-rpc-gateway/global"
)

// GatewayService 网关
//
// ============================================================================
// 网关里没有业务代码，一行都没有
// ============================================================================
//
// 它做四件事：
//   1. 按配置建网关
//   2. AutoRegisterWithHealthCheck —— 通过 gRPC server reflection 发现下游服务，
//      读取它们 proto 里的 google.api.http 注解，在**本进程里**建出 HTTP 路由
//   3. 挂一个鉴权中间件（见 middleware.go）
//   4. 重建 HTTP 网关让中间件生效，然后跑
//
// 第 2 步是为什么这个文件这么短：**没有任何 per-service 的注册代码**。
// 加一个下游服务只需要在 resources 的 grpc.clients 里加一条，
// 不用改这里，也不用 import 它的 pb 包。
//
// 代价与前提：
//   下游必须开 gRPC reflection（enable-reflection: true）。关掉的现象是
//   「接口 404 但服务明明活着」，而且网关启动日志里只会少几行，很容易漏看。
//
//   HTTP 路由来自 proto 注解，所以没有注解的 RPC 不会有 HTTP 入口。
//   这是特性不是缺陷：AuthService.Authorities 就刻意不加注解，
//   于是它永远拿不到 HTTP 路由（见 middleware.go 的说明）。
//
// [EN] The gateway holds no business code at all. It builds itself from config, calls
// AutoRegisterWithHealthCheck to discover downstream services through gRPC server reflection
// and build their HTTP routes inside this process from their proto annotations, attaches one
// authorization middleware and rebuilds the HTTP gateway so the middleware takes effect. Step
// two is why this file is short: there is no per-service registration code, and adding a
// downstream service means one entry under grpc.clients rather than a change here or an import
// of its pb package. The premise is that downstream services enable reflection, whose absence
// shows up as a 404 for a live service with only a few missing lines in the startup log; and
// that routes come from annotations, so an RPC without one has no HTTP entrance — a feature
// rather than a gap, since Authorities deliberately omits it.
type GatewayService struct {
	gateway *gateway.Gateway

	// healthChecker 下游 gRPC 连接的健康检查器
	//
	// 传给 AutoRegisterWithHealthCheck 之后，连接池会给每个下游挂上健康检查，
	// 并在客户端拦截器里对不健康的下游短路返回——不耗一次完整超时。
	// 这对网关尤其重要：一个挂掉的下游如果每次都要等超时，
	// 网关的连接和 goroutine 会被它慢慢占满，最后拖垮对其它下游的转发。
	//
	// [EN] Passed to AutoRegisterWithHealthCheck so the pool health-checks each downstream and
	// short-circuits an unhealthy one in the client interceptor instead of spending a full
	// timeout. That matters most in a gateway: a dead downstream that costs a timeout per
	// request slowly consumes its connections and goroutines until forwarding to the healthy
	// ones suffers too.
	healthChecker *grpcpool.HealthChecker
}

// NewGatewayService 创建网关实例
func NewGatewayService() *GatewayService {
	return &GatewayService{
		healthChecker: grpcpool.NewHealthChecker(),
	}
}

// Run 启动网关
func (g *GatewayService) Run() error {
	gw, err := gateway.NewGateway().
		WithSearchPath("resources").
		WithEnvironment(goconfig.GetEnvironment()).
		WithPrefix(constants.ServiceName).
		WithHotReload(nil).
		Build()
	if err != nil {
		return fmt.Errorf("创建网关失败 | create gateway failed: %w", err)
	}
	g.gateway = gw

	configureLoggerContext()

	// 自动注册：通过 reflection 发现下游并建出 HTTP 路由
	//
	// 返回值里有发现结果的摘要。打出来是必要的——下游名字拼错、reflection 没开、
	// 下游还没起来，这三种情况都不会让网关启动失败，只会让某些路由不存在。
	// 不看这行日志，排查时会从「接口 404」一路猜到网络策略。
	//
	// [EN] The result carries a summary and logging it is necessary: a misspelled downstream
	// name, disabled reflection and a downstream that has not started yet all leave the gateway
	// running with some routes missing rather than failing, and without this line an
	// investigation starts from a 404 and works its way to network policy.
	gwglobal.LOGGER.Info("正在通过 reflection 发现下游服务 | discovering downstream services...")
	result := g.gateway.AutoRegisterWithHealthCheck(g.healthChecker)
	gwglobal.LOGGER.Info("下游发现结果 | discovery result: %s", result.Summary())

	// 鉴权中间件
	//
	// 必须在 RebuildHTTPGateway 之前挂上：中间件是在重建时被织进 mux 的，
	// 之后再挂不会生效，而现象是**所有接口都免鉴权且一切正常**——
	// 这是整个网关最危险的一种配置错误，没有任何报错。
	//
	// [EN] Must be attached before RebuildHTTPGateway, which is when middleware is woven into
	// the mux; attaching it afterwards silently does nothing and leaves every endpoint
	// credential-free with everything apparently working, the most dangerous misconfiguration
	// here and one that raises no error.
	gwglobal.LOGGER.Info("正在挂载鉴权中间件 | attaching the authorization middleware...")
	g.gateway.AddGrpcGatewayMiddlewareProvider(g.getMiddleware)

	gwglobal.LOGGER.Info("正在重建 HTTP 网关 | rebuilding the HTTP gateway...")
	if err := g.gateway.RebuildHTTPGateway(); err != nil {
		return fmt.Errorf("重建 HTTP 网关失败 | rebuild http gateway failed: %w", err)
	}

	gwglobal.LOGGER.Info("GatewayService 已就绪 | ready")
	return g.gateway.Run()
}
