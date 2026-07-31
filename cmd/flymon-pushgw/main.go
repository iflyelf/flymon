// Command flymon-pushgw 是 flymon 的数据推送网关入口（对应上游 n9e-pushgw）。
package main

import (
	"github.com/ccfos/nightingale/v6/pushgw"

	"github.com/iflyelf/flymon/internal/bootstrap"
	"github.com/iflyelf/flymon/internal/cli"
)

func main() {
	opts := cli.Parse("N9E_PUSHGW_CONFIGS")

	bootstrap.Run("flymon-pushgw", opts.ConfigDir, opts.CryptoKey, pushgw.Initialize)
}
