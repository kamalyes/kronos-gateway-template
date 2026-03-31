/*
 * @Author: <% .AuthorName %> <% .AuthorEmail %>
 * @Date: <% nowFmt "2006-01-02" %> 00:00:00
 * @LastEditors: <% .AuthorName %> <% .AuthorEmail %>
 * @LastEditTime: <% nowFmt "2006-01-02" %> 00:00:00
 * @FilePath: \<% .ProjectName %>\main.go
 * @Description: 网关入口
 *
 * Copyright (c) <% .Year %> by <% .AuthorName %>, All Rights Reserved.
 */
package main

import (
	"log"

	"github.com/<% .OrgName %>/<% .ProjectName %>/bootstrap"
)

func main() {
	if err := bootstrap.New<% .ServiceName %>().Run(); err != nil {
		log.Fatalf("Gateway startup failed: %v", err)
	}
}
