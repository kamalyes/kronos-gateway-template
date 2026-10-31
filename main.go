/*
 * @Author: kamalyes example@qq.com
 * @Date: 2026-10-31 00:00:00
 * @LastEditors: kamalyes example@qq.com
 * @LastEditTime: 2026-10-31 00:00:00
 * @FilePath: \kronos-gateway-service\main.go
 * @Description: 网关入口
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package main

import (
	"log"

	"github.com/kamalyes/kronos-gateway-service/bootstrap"
)

func main() {
	if err := bootstrap.NewGatewayService().Run(); err != nil {
		log.Fatalf("Gateway startup failed: %v", err)
	}
}
