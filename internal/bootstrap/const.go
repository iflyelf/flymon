package bootstrap

import "time"

// defaultForceQuitDuration 是收到退出信号后，等待优雅关闭的最长时间，
// 超过则强制退出。与上游默认行为保持一致的宽松值。
const defaultForceQuitDuration = 30 * time.Second
