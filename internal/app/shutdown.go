package app

import (
	"context"
	"time"
)

func shutdown(cancel context.CancelFunc, waitForTerminals func(), shutdownHTTP func(context.Context) error, budget time.Duration) error {
	deadline := time.Now().Add(budget)
	cancel()

	// 必须先等 SSH Session 完整回收，再让 HTTP 关闭使用同一预算的剩余时间。
	waitForTerminals()
	ctx, stop := context.WithDeadline(context.Background(), deadline)
	defer stop()
	return shutdownHTTP(ctx)
}
