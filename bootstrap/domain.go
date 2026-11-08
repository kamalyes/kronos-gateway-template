/*
 * @Author: kamalyes example@qq.com
 * @Date: 2026-11-08 00:00:00
 * @LastEditors: kamalyes example@qq.com
 * @LastEditTime: 2026-11-08 00:00:00
 * @FilePath: \kronos-gateway-service\bootstrap\domain.go
 * @Description: 本网关服务的用户域
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package bootstrap

import (
	enumspb "github.com/kamalyes/kronos-share-proto-template/pb/enums"
)

// gatewayDomain 本网关固定服务的用户域
//
// 一个网关只服务一个域。要同时服务运维侧和租户侧，部署两个网关实例、
// 各自配一份 resources，而不是在一个进程里按路径分流——分流规则会被改，
// 而改错的后果是跨域越权，且现象是「某些接口突然能访问了」，不会报错。
//
// 这个值在编译期固定（由 kopy 变量渲染），不从配置读：配置可以热更新，
// 而「这个网关是哪一侧的」是部署决策，不该能在运行时被改掉。
//
// [EN] One gateway serves one domain. Serving both the ops and tenant sides means two
// deployments with their own resources rather than routing by path inside one process: a
// routing rule gets changed, and getting it wrong is a cross-domain privilege escalation whose
// symptom is some endpoints becoming reachable, with no error. The value is fixed at compile
// time from a kopy variable rather than read from config, because config is hot-reloadable
// while which side a gateway serves is a deployment decision that must not change at runtime.
const gatewayDomain = enumspb.DomainType_DOMAIN_TYPE_TENANT
