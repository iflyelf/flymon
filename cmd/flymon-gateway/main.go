// Command flymon-gateway 是 flymon 的飞书告警回调网关入口。
//
// 负责飞书卡片交互回调（屏蔽按钮）、AI 告警分析、协同群创建等功能，
// 原为独立 Go 服务 n9e-gateway，现内置为 flymon 可选子命令。
package main

import (
	"github.com/iflyelf/flymon/internal/bootstrap"
	"github.com/iflyelf/flymon/internal/cli"
	"github.com/iflyelf/flymon/internal/gateway"
)

func main() {
	opts := cli.Parse("N9E_CONFIGS")

	bootstrap.Run("flymon-gateway", opts.ConfigDir, opts.CryptoKey, gateway.Initialize)
}
