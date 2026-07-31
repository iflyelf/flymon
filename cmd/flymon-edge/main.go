// Command flymon-edge 是 flymon 的边缘节点入口（对应上游 n9e-edge）。
package main

import (
	"github.com/iflyelf/flymon/internal/bootstrap"
	"github.com/iflyelf/flymon/internal/cli"
	"github.com/iflyelf/flymon/internal/edge"
)

func main() {
	opts := cli.Parse("N9E_EDGE_CONFIGS")

	bootstrap.Run("flymon-edge", opts.ConfigDir, opts.CryptoKey, edge.Initialize)
}
