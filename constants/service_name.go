/*
 * @Author: kamalyes example@qq.com
 * @Date: 2026-11-08 00:00:00
 * @LastEditors: kamalyes example@qq.com
 * @LastEditTime: 2026-11-08 00:00:00
 * @FilePath: \kronos-gateway-service\constants\service_name.go
 * @Description: 服务名与下游服务名常量
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

package constants

// ServiceName 本网关的服务名，同时是配置文件前缀
//
// go-config 按 <前缀>-<环境别名>.yaml 匹配，bootstrap 用 WithPrefix(ServiceName)
// 取它，resources/ 下的文件名由同一个 kopy 变量渲染，所以两者不可能分叉。
//
// [EN] Also the config prefix: go-config matches <prefix>-<env alias>.yaml, bootstrap passes
// this to WithPrefix and the file name under resources/ renders from the same kopy variable.
const ServiceName = "kronos-gateway-service"

// 下游服务名 | Downstream service names
//
// 这些字符串必须与三处完全一致，差一个字符就静默失效：
//   1. resources/kronos-gateway-service-dev.yaml 里 grpc.clients 的 key
//   2. 对应服务自己的 constants.ServiceName
//   3. 这里
//
// 连接池按服务名索引。对不上时 grpcpool.GetConn 返回 ok=false，
// 而网关的鉴权中间件会把它报成 "auth service connection is nil" ——
// 看起来像 AC 服务挂了，实际是名字拼错了。
//
// [EN] These strings must match the keys under grpc.clients in the resources file and each
// service's own ServiceName exactly. The pool is indexed by service name, so a mismatch makes
// GetConn return false and the middleware report a nil auth connection, which looks like the
// authorization service being down rather than a typo.
const (
	// AccessControlServiceName 鉴权服务
	//
	// 网关对它的依赖和对其它下游不同：其它下游是「被代理的对象」，
	// 它是「代理的前置条件」。它不可用时网关无法判断任何请求该不该放行，
	// 所以中间件会直接返回 500 而不是放行——放行等于在鉴权服务挂掉的
	// 几分钟里把所有接口变成公开接口。
	//
	// [EN] The gateway depends on it differently from the services it proxies: those are the
	// objects of proxying while this is its precondition. When it is unavailable the gateway
	// cannot decide anything, so the middleware returns 500 rather than allowing through, since
	// allowing would turn every endpoint public for however long the outage lasts.
	AccessControlServiceName = "kronos-access-control-service"

	// BusinessServiceName 业务服务（由 kronos-service-template 生成）
	// [EN] The business service generated from kronos-service-template
	BusinessServiceName = "kronos-my-service"

	// KeyServiceName 密钥与发号服务（由 kronos-key-service-template 生成）
	//
	// 注意网关代理它的 HTTP 路由，而它自己不注册 HTTP handler——
	// 路由是网关通过 gRPC server reflection 读取 proto 注解后在本进程里建出来的。
	// 所以密钥接口的唯一入口是网关，必然经过鉴权。
	//
	// [EN] The gateway proxies its HTTP routes although it registers no handler of its own: the
	// routes are built here from the proto annotations discovered through gRPC server
	// reflection, so the only entrance to the key endpoints is the gateway and authorization is
	// unavoidable.
	KeyServiceName = "kronos-key-service"
)
