/*
 * @Author: kamalyes example@qq.com
 * @Date: 2026-10-31 00:00:00
 * @LastEditors: kamalyes example@qq.com
 * @LastEditTime: 2026-10-31 00:00:00
 * @FilePath: \kronos-gateway-service\bootstrap\middleware.go
 * @Description: 鉴权中间件 —— 网关唯一的业务逻辑
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package bootstrap

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/kamalyes/kronos-gateway-service/constants"
	authpb "github.com/kamalyes/kronos-share-proto-template/pb/access_control"
	commonpb "github.com/kamalyes/kronos-share-proto-template/pb/common"
	enumspb "github.com/kamalyes/kronos-share-proto-template/pb/enums"
	gruntime "github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	goi18n "github.com/kamalyes/go-i18n"
	grpcpool "github.com/kamalyes/go-rpc-gateway/cpool/grpc"
	gwglobal "github.com/kamalyes/go-rpc-gateway/global"
	gwmiddleware "github.com/kamalyes/go-rpc-gateway/middleware"
	"github.com/kamalyes/go-rpc-gateway/response"
	"github.com/kamalyes/go-toolbox/pkg/convert"
)

// authCheckTimeout 单次鉴权调用的超时
//
// 必须有，而且不能太长。鉴权是**每个请求**的前置步骤，它的延迟直接加在
// 所有接口上。AC 服务重启或网络抖动时，没有超时会让网关的请求全部堆在
// 等待鉴权上，连接和 goroutine 被占满，最后连健康的下游也转发不动。
//
// 也不能太短：AC 服务冷启动、策略缓存首次加载都需要时间，超时太短会在
// 发布窗口里把所有请求判成 500。十几秒量级是常见折中。
//
// [EN] Mandatory and not too long: authorization precedes every request and its latency is
// added to every endpoint, so without a timeout a restart or a network blip leaves every
// gateway request waiting on it until connections and goroutines are exhausted and even healthy
// downstreams cannot be reached. Not too short either, since a cold start and a first policy
// load take time and an aggressive timeout turns every request into a 500 during a release.
const authCheckTimeout = 15 * time.Second

// getMiddleware 返回网关的中间件链
//
// ============================================================================
// 这是整个网关唯一的业务逻辑
// ============================================================================
//
// 流程：
//   1. 从连接池拿 AC 服务的连接（AutoRegister 已经按配置建好了）
//   2. 调 Authorities(path, method)，把请求的 metadata 整体透传过去
//   3. 拒绝 → 按 token_valid 决定 401 还是 403
//   4. 放行 → 把返回的 Payload 注入 HTTP header 与 context，交给下一环
//
// 第 4 步是这套架构的关键：下游服务**不重复验 token**，它们从 header 里读身份。
// 所以这里注入什么，下游就信什么——这也意味着下游服务绝不能直接暴露给外网。
//
// [EN] The gateway's only business logic: take the authorization service's connection from the
// pool, call Authorities with the request's metadata forwarded wholesale, answer 401 or 403 on
// a denial according to token_valid, and on success inject the returned payload into the HTTP
// headers and context for the next hop. That last step is the architecture's crux, because
// downstream services do not re-verify the token and read identity from the header, so whatever
// is injected here is believed — which is also why they must never be exposed directly.
func (g *GatewayService) getMiddleware() []gruntime.Middleware {
	return []gruntime.Middleware{
		func(next gruntime.HandlerFunc) gruntime.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request, pathParams map[string]string) {
				// 从连接池取连接，不自己拨号
				//
				// AutoRegister 已经按 resources 里的 grpc.clients 建好了连接，
				// 并挂上了健康检查与链路透传拦截器。自己 grpc.NewClient 会
				// 绕开这些，而且每个请求建一条连接。
				//
				// 取不到说明服务名对不上（见 constants 里的三处一致说明）或者
				// 配置里没有这条 client。这里报 500 而不是放行——放行等于
				// 在鉴权不可用的时候把所有接口变成公开接口。
				//
				// [EN] The connection comes from the pool, already built from grpc.clients with
				// health checking and propagation interceptors attached; dialing here would
				// bypass both and open a connection per request. A miss means the service name
				// does not match or the client is absent from config, and the answer is 500
				// rather than passing through, since passing through would make every endpoint
				// public exactly while authorization is unavailable.
				conn, ok := grpcpool.GetConn(constants.AccessControlServiceName)
				if !ok || conn == nil {
					gwglobal.LOGGER.ErrorContextKV(r.Context(),
						"取不到鉴权服务连接 | no connection to the authorization service",
						"service", constants.AccessControlServiceName)
					response.WriteInternalServerErrorResult(w,
						goi18n.T(r.Context(), "error.ac_service_unavailable"))
					return
				}
				authClient := authpb.NewAuthServiceClient(conn)

				// ForwardOutgoingContext 把 HTTP 请求头整体转成出向 gRPC metadata
				//
				// 这是 token 到达 AC 服务的方式——协议里刻意没有 token 字段
				// （放请求体会进访问日志和链路 span，而那些地方的保留期
				// 远长于 token 有效期）。
				//
				// [EN] Turns the HTTP headers into outgoing gRPC metadata, which is how the
				// token reaches the authorization service: the contract deliberately has no
				// token field, because a body field lands in access logs and trace spans whose
				// retention far outlives the token.
				authCtx := gwmiddleware.ForwardOutgoingContext(r)
				authCtx, cancel := context.WithTimeout(authCtx, authCheckTimeout)
				defer cancel()

				resp, err := authClient.Authorities(authCtx, &authpb.AuthoritiesRequest{
					Policy: &authpb.AuthoritiesPolicy{
						V1: r.URL.Path,
						V2: r.Method,
					},
				})
				if err != nil {
					// 调用失败一律 500，不要放行也不要 401
					//
					// 放行等于鉴权不可用时全站公开。401 则会误导客户端去重新登录——
					// 而问题不在它的凭据上，重新登录也解决不了，只会在故障期间
					// 给登录接口再加一波流量。
					//
					// [EN] A failure is always 500: passing through would open the whole site
					// while authorization is down, and a 401 would mislead the client into
					// logging in again, which cannot help since the credentials are not the
					// problem and only adds load to the login endpoint during an incident.
					gwglobal.LOGGER.WarnContextKV(r.Context(), "鉴权调用失败 | authorization call failed",
						"path", r.URL.Path, "method", r.Method, "error", err)
					response.WriteInternalServerErrorResult(w,
						goi18n.T(r.Context(), "error.ac_service_unavailable"))
					return
				}

				if !resp.GetAllowed() {
					// token_valid 决定 401 还是 403，两者对客户端含义完全不同：
					//   false → 未认证（缺失/过期/伪造）→ 401 → 该去重新登录
					//   true  → 已认证但权限不足        → 403 → 重新登录也没用
					// 混为一谈会让前端在权限不足时反复跳登录页，用户陷在登录循环里。
					//
					// [EN] token_valid decides between 401 and 403, which mean different things
					// to a client: unauthenticated should log in again, while insufficiently
					// authorized gains nothing from it. Conflating them traps the user in a
					// login loop on every permission error.
					gwglobal.LOGGER.WarnContextKV(r.Context(), "鉴权拒绝 | authorization denied",
						"path", r.URL.Path, "method", r.Method,
						"token_valid", resp.GetTokenValid(), "reason", resp.GetReason())
					if resp.GetTokenValid() {
						response.WriteForbiddenResult(w, resp.GetReason())
						return
					}
					response.WriteUnauthorizedResult(w, resp.GetReason())
					return
				}

				payload := resp.GetPayload()
				if payload == nil {
					payload = &commonpb.Payload{}
				}

				// 校验并钉死域
				//
				// 本网关固定服务一个域（由 kopy 变量决定）。校验 AC 返回的域与之一致
				// 这一道比它看起来重要：运维网关和租户网关通常共用同一个 AC 服务，
				// 不校验的话，一个租户用户的 token 拿到运维网关上去用，
				// AC 会认为它合法（token 确实有效），于是租户用户越权进了运维接口。
				//
				// [EN] This gateway serves one domain and verifies that the one returned matches.
				// That check matters more than it looks: an ops gateway and a tenant gateway
				// usually share one authorization service, and without it a tenant user's token
				// presented to the ops gateway is accepted, since the token really is valid, and
				// that user reaches ops endpoints.
				if !g.ensurePayloadDomain(w, r, payload) {
					return
				}

				injectPayloadToHeaders(r, payload)
				r = injectPayloadToContext(r, payload)

				next(w, r, pathParams)
			}
		},
	}
}

// ensurePayloadDomain 校验并钉死 Payload 的域
func (g *GatewayService) ensurePayloadDomain(
	w http.ResponseWriter, r *http.Request, payload *commonpb.Payload,
) bool {
	expected := gatewayDomain
	expectedIsOps := expected == enumspb.DomainType_DOMAIN_TYPE_OPS

	// 只在有 user_id 时校验：免鉴权路径放行时 Payload 是空的，
	// 空 Payload 的域当然不匹配，但那不是越权
	// [EN] Only checked when a user id is present: an allow-listed path passes with an empty
	// payload whose domain naturally does not match, which is not a privilege problem
	if payload.GetUserId() != "" &&
		(payload.GetDomain() != expected || payload.GetIsOps() != expectedIsOps) {
		gwglobal.LOGGER.WarnContextKV(r.Context(), "Payload 域不匹配 | payload domain mismatch",
			"expected", expected.String(), "actual", payload.GetDomain().String(),
			"user_id", payload.GetUserId())
		response.WriteForbiddenResult(w, goi18n.T(r.Context(), "error.permission_denied"))
		return false
	}

	// 钉死而不是沿用 AC 返回的值：本网关只服务这一个域，
	// 下游据此做的任何判断都应该以网关的身份为准
	// [EN] Pinned rather than taken from the response: this gateway serves one domain and any
	// downstream decision should rest on the gateway's identity
	payload.Domain = expected
	payload.IsOps = expectedIsOps
	return true
}

// injectPayloadToHeaders 把 Payload 写进 HTTP header
//
// 用 go-rpc-gateway 的 RequestContext header 常量，不写字面量：
// 下游的 request_context 中间件按同一批常量读取，两边各写字面量
// 就会在改名时静默失配——现象是下游拿到空身份，而租户隔离等于失效。
//
// [EN] Uses the gateway's own RequestContext header constants rather than literals, because the
// downstream request_context middleware reads the same ones; literals on both sides silently
// diverge on a rename, leaving downstream with an empty identity and tenant isolation void.
func injectPayloadToHeaders(r *http.Request, payload *commonpb.Payload) {
	keys := gwglobal.GATEWAY.RequestContext.GetSourceKeys()
	r.Header.Set(keys.UserID.Header, payload.GetUserId())
	r.Header.Set(keys.Domain.Header, payload.GetDomain().String())
	r.Header.Set(keys.RoleCode.Header, payload.GetRoleCode())
	r.Header.Set(keys.TenantID.Header, payload.GetTenantId())
	r.Header.Set(keys.RegionCode.Header, payload.GetRegionCode())
	r.Header.Set(keys.Jti.Header, payload.GetJti())
	r.Header.Set(keys.FamilyId.Header, payload.GetFamilyId())

	// 完整 Payload 走 Base64
	//
	// 必须编码：昵称等字段可能含非 ASCII 字符，而 gRPC metadata 的值
	// 只允许 ASCII（非 ASCII 会让整个请求在 metadata 校验阶段失败）。
	// 编码失败就不设这个 header，上面那些单字段仍然有效——不要因为
	// 一个可选的聚合字段让整个请求失败。
	//
	// [EN] Encoding is mandatory: a nickname may hold non-ASCII characters while gRPC metadata
	// values must be ASCII, and a non-ASCII value fails the whole request at metadata
	// validation. A failed encode simply omits this header and leaves the individual fields
	// above intact, rather than failing a request over an optional aggregate.
	if raw, err := json.Marshal(payload); err == nil {
		if encoded, encErr := convert.B64Encode(raw); encErr == nil {
			r.Header.Set(keys.AuthPayload.Header, encoded)
		}
	}
}

// injectPayloadToContext 把 Payload 写进请求 context
//
// header 和 context 都要写：header 给下游服务（跨进程），context 给本进程里
// 后续的中间件和日志。只写 header 的话，网关自己的访问日志里没有用户身份。
//
// [EN] Both are written: the header serves downstream services across the process boundary and
// the context serves this process's later middleware and logs. Header only would leave the
// gateway's own access log without the caller's identity.
func injectPayloadToContext(r *http.Request, payload *commonpb.Payload) *http.Request {
	ctx := r.Context()
	ctx = gwmiddleware.WithUserID(ctx, payload.GetUserId())
	ctx = gwmiddleware.WithUserName(ctx, payload.GetNickname())
	ctx = gwmiddleware.WithDomain(ctx, payload.GetDomain().String())
	ctx = gwmiddleware.WithRoleCode(ctx, payload.GetRoleCode())
	ctx = gwmiddleware.WithTenantID(ctx, payload.GetTenantId())
	ctx = gwmiddleware.WithRegionCode(ctx, payload.GetRegionCode())
	ctx = gwmiddleware.WithJti(ctx, payload.GetJti())
	ctx = gwmiddleware.WithFamilyId(ctx, payload.GetFamilyId())
	return r.WithContext(ctx)
}
