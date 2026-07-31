// Package cli 提供 flymon 各入口共享的命令行参数解析逻辑。
package cli

import (
	"flag"
	"fmt"
	"os"

	"github.com/ccfos/nightingale/v6/pkg/version"

	"github.com/toolkits/pkg/runner"
)

// Options 保存解析后的启动参数。
type Options struct {
	ConfigDir string
	CryptoKey string
}

// Parse 解析命令行参数。envKey 用于兼容上游各服务不同的配置目录环境变量
// （N9E_CONFIGS / N9E_EDGE_CONFIGS / N9E_PUSHGW_CONFIGS）。
// 若指定了 --version 则打印版本号后退出。
func Parse(envKey string) Options {
	var (
		showVersion = flag.Bool("version", false, "Show version.")
		configDir   = flag.String("configs", getEnv(envKey, "etc"), fmt.Sprintf("Specify configuration directory.(env:%s)", envKey))
		cryptoKey   = flag.String("crypto-key", "", "Specify the secret key for configuration file field encryption.")
	)
	flag.Parse()

	if *showVersion {
		fmt.Println(version.Version)
		os.Exit(0)
	}

	printEnv()

	return Options{
		ConfigDir: *configDir,
		CryptoKey: *cryptoKey,
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func printEnv() {
	runner.Init()
	fmt.Println("flymon.version:", version.Version)
	fmt.Println("runner.cwd:", runner.Cwd)
	fmt.Println("runner.hostname:", runner.Hostname)
	fmt.Println("runner.fd_limits:", runner.FdLimits())
	fmt.Println("runner.vm_limits:", runner.VMLimits())
}
