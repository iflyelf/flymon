// Command flymon 是 flymon 的中心服务入口（对应上游 n9e / center）。
//
// 基于 go-zero 包装层启动：命令行解析、版本注入、优雅退出均由
// internal/cli 与 internal/bootstrap 统一处理，业务初始化复用上游
// center.Initialize（含事件聚合补丁）。
package main

import (
	"github.com/ccfos/nightingale/v6/center"

	"github.com/toolkits/pkg/net/tcpx"

	"github.com/iflyelf/flymon/internal/bootstrap"
	"github.com/iflyelf/flymon/internal/cli"
)

func main() {
	opts := cli.Parse("N9E_CONFIGS")

	// 中心服务依赖数据库/Redis，等待依赖就绪（与上游行为一致）。
	tcpx.WaitHosts()

	bootstrap.Run("flymon-center", opts.ConfigDir, opts.CryptoKey, center.Initialize)
}
