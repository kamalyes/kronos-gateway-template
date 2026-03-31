/*
 * @Author: <% .AuthorName %> <% .AuthorEmail %>
 * @Date: <% nowFmt "2006-01-02" %> 00:00:00
 * @LastEditors: <% .AuthorName %> <% .AuthorEmail %>
 * @LastEditTime: <% nowFmt "2006-01-02" %> 00:00:00
 * @FilePath: \<% .ProjectName %>\bootstrap\logger.go
 * @Description: 日志上下文字段配置
 *
 * Copyright (c) <% .Year %> by <% .AuthorName %>, All Rights Reserved.
 */
package bootstrap

import (
	logger "github.com/kamalyes/go-logger"
	gwconstants "github.com/kamalyes/go-rpc-gateway/constants"
	gwglobal "github.com/kamalyes/go-rpc-gateway/global"
)

// configureLoggerContext 声明每条 Context 日志自动带出哪些字段
//
// 配一次，之后所有 *ContextKV / *Context 日志都自动带上这些字段，业务代码里
// 一个字都不用写——不要在调用处手工拼 request_id / tenant_id。
//
// go-logger 的取值顺序（见 extractKeyValue）：
//  1. trace_id 固定从 OTel span 取，是单一真相源，不受这里的配置影响；
//     网关的 gRPC server 挂了 otelgrpc 的 StatsHandler，所以每个 RPC 的 ctx 都在 span 内。
//  2. 其余 key 先查 ctx.Value(key)——网关的 request_context 中间件正是用
//     constants.Metadata* 这些字符串做键存进 ctx 的，所以这里直接用同一批常量，
//     不要写字面量 "request_id"：那个键没人往里存，查出来恒为空。
//  3. ctx 里没有则回落 gRPC incoming metadata 的同名 header。
//
// 注意日志里的字段名就是这里的 key，即 "x-tenant-id" 而不是 "tenant_id"：
// 查找键和输出键是同一个，好处是与 HTTP header、gRPC metadata 三处同名。
//
// 对密钥服务额外重要：这些字段是事后审计「谁在什么时候签发/解密了哪个密钥」
// 的唯一线索，而密钥本身不会进日志。
//
// [EN] Declare once which fields every context log carries. go-logger resolves trace_id from
// the OTel span as the single source of truth, and for the remaining keys reads ctx.Value(key)
// first and then falls back to the gRPC incoming metadata header of the same name; the
// gateway's request_context middleware stores its values under exactly these constants, which
// is why they are used rather than literals. The field names are the keys themselves, so logs,
// HTTP headers and gRPC metadata all agree. This matters more for a key service: these fields
// are the only trail for auditing who issued or decrypted which key and when, since the key
// material itself never reaches a log.
func configureLoggerContext() {
	// WithContextKeys 只在具体类型 *logger.Logger 上，不在 ILogger 接口上
	// [EN] WithContextKeys lives on the concrete *logger.Logger, not on the ILogger interface
	l, ok := gwglobal.LOGGER.(*logger.Logger)
	if !ok {
		return
	}
	l.WithContextKeys(
		logger.ContextKeyTraceID,
		gwconstants.MetadataRequestID,
		gwconstants.MetadataTenantID,
		gwconstants.MetadataUserID,
	)
}
