// Package bootstrap 提供 flymon 各服务基于 go-zero 的启动封装。
//
// 设计说明：
// 夜莺（nightingale）上游是一个成熟的大型 Go 项目，其 center/pushgw 的
// Initialize 已是导出函数。flymon 采用 go-zero 包装层方案：
//   - 使用 go-zero 的 service.ServiceGroup 统一管理服务生命周期
//   - 使用 go-zero 的 proc 优雅退出（信号处理、shutdown 钩子）
//   - 使用 go-zero 的 logx 输出统一启动日志
//
// 这样既保持 go-zero 架构风格，又能让底层业务逻辑跟随上游升级。
package bootstrap

import (
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/proc"
	"github.com/zeromicro/go-zero/core/service"
)

// InitFunc 是上游各模块的初始化函数签名（center/pushgw/edge 一致）。
// 返回一个 cleanup 函数用于释放资源。
type InitFunc func(configDir string, cryptoKey string) (func(), error)

// nightingaleService 将上游服务包装为 go-zero 的 service.Service 接口。
type nightingaleService struct {
	name      string
	configDir string
	cryptoKey string
	init      InitFunc
	cleanup   func()
}

// Start 实现 service.Service，调用上游 Initialize 完成真正的服务启动。
// 上游 Initialize 内部已启动 HTTP server 与各类后台协程（非阻塞），
// 因此这里返回后由 ServiceGroup 交给 proc 阻塞等待退出信号。
func (s *nightingaleService) Start() {
	logx.Infof("[flymon] starting service %q, configs=%s", s.name, s.configDir)

	cleanup, err := s.init(s.configDir, s.cryptoKey)
	if err != nil {
		logx.Errorf("[flymon] failed to initialize %q: %v", s.name, err)
		panic(err)
	}
	s.cleanup = cleanup

	logx.Infof("[flymon] service %q started", s.name)
}

// Stop 实现 service.Service，触发上游 cleanup 释放资源。
func (s *nightingaleService) Stop() {
	logx.Infof("[flymon] stopping service %q", s.name)
	if s.cleanup != nil {
		s.cleanup()
	}
	logx.Infof("[flymon] service %q stopped", s.name)
}

// Run 使用 go-zero 的 ServiceGroup 启动指定服务并阻塞直到收到退出信号。
func Run(name, configDir, cryptoKey string, init InitFunc) {
	logx.DisableStat()

	group := service.NewServiceGroup()
	defer group.Stop()

	group.Add(&nightingaleService{
		name:      name,
		configDir: configDir,
		cryptoKey: cryptoKey,
		init:      init,
	})

	// go-zero 的 ServiceGroup.Start 会阻塞，内部通过 proc 监听
	// SIGTERM/SIGINT，收到后依次调用各 service 的 Stop。
	proc.SetTimeToForceQuit(defaultForceQuitDuration)
	group.Start()
}
