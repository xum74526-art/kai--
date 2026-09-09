package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"infinite-canvas/backend/internal/model"
)

var errWorkerShuttingDown = errors.New("后台服务正在关闭")

// taskWorkerLifecycle 把“停止领取”和“取消在途任务”拆成两个阶段。
// 先停止调度并等待自然完成；只有排空窗口即将耗尽时才取消在途请求，
// 让任务租约过期后由下一实例依据上游 ID/提交状态安全恢复。
type taskWorkerLifecycle struct {
	stateMu sync.Mutex
	claimMu sync.Mutex

	started   bool
	accepting bool
	loopCtx   context.Context
	stopLoops context.CancelFunc
	taskCtx   context.Context
	stopTasks context.CancelCauseFunc

	loopWG sync.WaitGroup
	taskWG sync.WaitGroup
}

func newTaskWorkerLifecycle() *taskWorkerLifecycle {
	return &taskWorkerLifecycle{}
}

func (s *Service) lifecycle() *taskWorkerLifecycle {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	if s.workerLifecycle == nil {
		s.workerLifecycle = newTaskWorkerLifecycle()
	}
	return s.workerLifecycle
}

func (l *taskWorkerLifecycle) start() bool {
	l.stateMu.Lock()
	defer l.stateMu.Unlock()
	if l.started {
		return false
	}
	l.loopCtx, l.stopLoops = context.WithCancel(context.Background())
	l.taskCtx, l.stopTasks = context.WithCancelCause(context.Background())
	l.started = true
	l.accepting = true
	return true
}

func (l *taskWorkerLifecycle) goLoop(run func(context.Context)) bool {
	l.stateMu.Lock()
	if !l.started || !l.accepting || l.loopCtx == nil {
		l.stateMu.Unlock()
		return false
	}
	ctx := l.loopCtx
	l.loopWG.Add(1)
	l.stateMu.Unlock()
	go func() {
		defer l.loopWG.Done()
		run(ctx)
	}()
	return true
}

// claimTask 与 beginShutdown 共用 claimMu，保证关闭边界之后不会再领取任务。
// 已经领取成功的任务会在释放该锁前计入 taskWG，因而属于需要排空的在途任务。
func (l *taskWorkerLifecycle) claimTask(claim func() (*model.Task, error)) (*model.Task, context.Context, func(), error) {
	l.claimMu.Lock()
	defer l.claimMu.Unlock()

	l.stateMu.Lock()
	if !l.started || !l.accepting || l.taskCtx == nil {
		l.stateMu.Unlock()
		return nil, nil, nil, errWorkerShuttingDown
	}
	// 在调用可能阻塞的数据库领取前登记。beginShutdown 会先在同一把
	// stateMu 下关闭 accepting，因此 Shutdown 开始 Wait 后不会再发生 Add。
	l.taskWG.Add(1)
	ctx := l.taskCtx
	l.stateMu.Unlock()

	task, err := claim()
	if err != nil || task == nil {
		l.taskWG.Done()
		return task, nil, nil, err
	}
	l.stateMu.Lock()
	accepting := l.accepting
	l.stateMu.Unlock()
	if !accepting {
		// 领取在关闭边界前已经进入数据库，但边界后才返回。保留数据库
		// 租约让下一实例恢复，不启动新的上游调用。
		l.taskWG.Done()
		return task, nil, nil, errWorkerShuttingDown
	}
	var once sync.Once
	done := func() { once.Do(l.taskWG.Done) }
	return task, ctx, done, nil
}

func (l *taskWorkerLifecycle) beginShutdown() {
	l.stateMu.Lock()
	defer l.stateMu.Unlock()
	if !l.started || !l.accepting {
		return
	}
	l.accepting = false
	if l.stopLoops != nil {
		l.stopLoops()
	}
}

func (l *taskWorkerLifecycle) isReady() bool {
	l.stateMu.Lock()
	defer l.stateMu.Unlock()
	return l.started && l.accepting
}

func (l *taskWorkerLifecycle) shutdown(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	l.beginShutdown()

	allDone := make(chan struct{})
	go func() {
		l.loopWG.Wait()
		l.taskWG.Wait()
		close(allDone)
	}()

	softTimer := time.NewTimer(workerDrainWindow(ctx))
	defer softTimer.Stop()
	select {
	case <-allDone:
		return nil
	case <-ctx.Done():
		l.cancelTasksForShutdown()
		return fmt.Errorf("等待后台任务停止：%w", ctx.Err())
	case <-softTimer.C:
		l.cancelTasksForShutdown()
	}

	select {
	case <-allDone:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("等待后台任务安全暂停：%w", ctx.Err())
	}
}

func (l *taskWorkerLifecycle) cancelTasksForShutdown() {
	l.stateMu.Lock()
	defer l.stateMu.Unlock()
	if l.stopTasks != nil {
		l.stopTasks(errWorkerShuttingDown)
	}
}

func workerDrainWindow(ctx context.Context) time.Duration {
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return 0
		}
		// 预留最后 20% 给请求取消、租约状态落盘和 goroutine 收尾。
		return remaining * 4 / 5
	}
	return 30 * time.Second
}

func workerShutdownCause(ctx context.Context) error {
	if err := context.Cause(ctx); errors.Is(err, errWorkerShuttingDown) {
		return err
	}
	return nil
}

// BeginShutdown 先把实例移出就绪状态并停止所有领取/清理循环。
func (s *Service) BeginShutdown() {
	s.lifecycle().beginShutdown()
}

// Shutdown 有界等待已经领取的任务；超出自然排空窗口后只取消本轮执行，
// 不写失败终态、不退款，任务由持久化租约与上游 ID 在重启后恢复。
func (s *Service) Shutdown(ctx context.Context) error {
	return s.lifecycle().shutdown(ctx)
}
