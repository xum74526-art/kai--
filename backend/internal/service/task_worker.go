package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"infinite-canvas/backend/internal/model"
)

// taskWorkerCoordinator 收敛任务领取、租约维护和执行结果落库，避免 Service 同时承担 worker 生命周期与业务命令。
type taskWorkerCoordinator struct {
	service *Service
}

const (
	workerGlobalSlotLeaseTTL       = 45 * time.Minute
	workerGlobalSlotRenewInterval  = 5 * time.Minute
	workerGlobalSlotRenewTimeout   = 5 * time.Second
	workerGlobalSlotRenewAttempts  = 3
	workerGlobalSlotRenewRetryWait = time.Second
)

func newTaskWorkerCoordinator(service *Service) *taskWorkerCoordinator {
	return &taskWorkerCoordinator{service: service}
}

func (s *Service) taskWorker() *taskWorkerCoordinator {
	if s.taskWorkerCoordinator != nil {
		return s.taskWorkerCoordinator
	}
	// 部分单元测试直接构造 Service 字面量；延迟创建保持这些测试和内部工具兼容。
	return newTaskWorkerCoordinator(s)
}

func (w *taskWorkerCoordinator) start() {
	s := w.service
	lifecycle := s.lifecycle()
	if !lifecycle.start() {
		return
	}
	s.startStagedGeneratedResourceCleanup()
	s.startTextReplayCleanup()
	s.startProviderCancellationReconciliation()
	s.startBillingReviewAudit()
	lifecycle.goLoop(func(loopCtx context.Context) {
		slots := make(chan struct{}, maxChannelConcurrencyLimit)
		dispatch := func() {
			if loopCtx.Err() != nil {
				return
			}
			setting, err := s.runtimeConcurrencySetting()
			if err != nil {
				return
			}
			workerConcurrency := setting.WorkerConcurrency
			for len(slots) < workerConcurrency {
				globalLease, acquired, err := s.coordinator.acquireLease(loopCtx, "workers", workerConcurrency, workerGlobalSlotLeaseTTL)
				if err != nil || !acquired {
					return
				}
				task, taskCtx, taskDone, err := lifecycle.claimTask(func() (*model.Task, error) {
					return s.repo.ClaimNextTask(s.workerID, 45*time.Second)
				})
				if err != nil || task == nil {
					globalLease.Release()
					return
				}
				slots <- struct{}{}
				go func(task *model.Task, taskCtx context.Context, taskDone func(), globalLease *runtimeSlotLease) {
					defer taskDone()
					executionCtx, cancelExecution := context.WithCancelCause(taskCtx)
					defer cancelExecution(nil)
					renewalDone := make(chan struct{})
					renewalStopped := make(chan struct{})
					go func() {
						defer close(renewalStopped)
						w.renewWorkerGlobalSlot(task, globalLease, renewalDone, func(err error) {
							// 不能在分布式槽位失效后继续消耗上游配额；本轮不写终态，
							// 让任务租约自然释放后由其他 worker 根据已有上游请求 ID 恢复。
							cancelExecution(fmt.Errorf("%w: %v", errRuntimeSlotLeaseLost, err))
						})
					}()
					defer func() {
						// 先归还跨实例额度，再释放本机 worker 名额，避免下一次调度白领本机槽位。
						globalLease.Release()
						<-slots
					}()
					defer func() {
						close(renewalDone)
						<-renewalStopped
					}()
					if err := w.processClaimedTaskWithContext(executionCtx, task); err != nil {
						if errors.Is(err, errWorkerShuttingDown) {
							_ = s.log(task.UserID, task.ID, "info", "实例关闭，本轮执行已安全暂停并等待恢复", task.ProviderRequestID)
							return
						}
						if errors.Is(err, errRuntimeSlotLeaseLost) {
							_ = s.log(task.UserID, task.ID, "warn", "后台全局并发槽位已失效，本轮停止并等待任务恢复", err.Error())
							return
						}
						_ = s.log(task.UserID, task.ID, "error", "后台任务处理失败", err.Error())
					}
				}(task, taskCtx, taskDone, globalLease)
			}
		}

		dispatch()
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-loopCtx.Done():
				return
			case <-ticker.C:
				dispatch()
			}
		}
	})
}

// renewWorkerGlobalSlot 与已领取任务同生共死：任务租约仍按原有 45 秒规则维护，
// 这里仅刷新跨实例 Worker 并发配额。续租长期不可用时必须中止本轮执行，
// 不能等旧 TTL 自然过期后继续运行而让其他实例重复占用同一全局额度。
func (w *taskWorkerCoordinator) renewWorkerGlobalSlot(task *model.Task, lease *runtimeSlotLease, done <-chan struct{}, onLeaseUnavailable func(error)) {
	ticker := time.NewTicker(workerGlobalSlotRenewInterval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			err := renewWorkerGlobalSlotLease(lease, done)
			if err == nil || errors.Is(err, context.Canceled) {
				continue
			}
			_ = w.service.log(task.UserID, task.ID, "warn", "后台全局并发槽位续租失败", err.Error())
			onLeaseUnavailable(err)
			return
		}
	}
}

func renewWorkerGlobalSlotLease(lease *runtimeSlotLease, done <-chan struct{}) error {
	var lastErr error
	for attempt := 0; attempt < workerGlobalSlotRenewAttempts; attempt++ {
		select {
		case <-done:
			return context.Canceled
		default:
		}
		ctx, cancel := context.WithTimeout(context.Background(), workerGlobalSlotRenewTimeout)
		err := lease.Renew(ctx)
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err
		if errors.Is(err, errRuntimeSlotLeaseLost) || attempt == workerGlobalSlotRenewAttempts-1 {
			return err
		}
		timer := time.NewTimer(workerGlobalSlotRenewRetryWait)
		select {
		case <-done:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return context.Canceled
		case <-timer.C:
		}
	}
	return lastErr
}

func (w *taskWorkerCoordinator) processNextTask() error {
	s := w.service
	task, err := s.repo.ClaimNextTask(s.workerID, 45*time.Second)
	if err != nil || task == nil {
		return err
	}
	return w.processClaimedTask(task)
}

func (w *taskWorkerCoordinator) processClaimedTask(task *model.Task) error {
	return w.processClaimedTaskWithContext(context.Background(), task)
}

func (w *taskWorkerCoordinator) processClaimedTaskWithContext(parent context.Context, task *model.Task) error {
	s := w.service
	terminal := s.terminalCoordinator()
	if parent == nil {
		parent = context.Background()
	}
	_ = s.log(task.UserID, task.ID, "info", "后端任务开始处理", "")
	// 进程可能在上游已受理、Task 尚未来得及写入请求 ID 时中断。
	// 先从账单或调用日志补齐，再按当前租约回写，避免重启后重复提交上游任务。
	if strings.TrimSpace(task.ProviderRequestID) == "" {
		s.hydrateTaskProviderRequestID(task)
		if strings.TrimSpace(task.ProviderRequestID) != "" {
			if err := s.repo.UpdateTaskProviderStateWithLease(task.ID, task.LeaseOwner, task.Attempts, task.ProviderRequestID, task.PollStage, task.NextPollAt); err != nil {
				return fmt.Errorf("恢复上游任务标识失败：%w", err)
			}
		}
	}
	if shutdownErr := workerShutdownCause(parent); shutdownErr != nil {
		return shutdownErr
	}
	policy, err := s.RuntimePolicy()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(parent, taskExecutionTimeoutWithPolicy(task.Type, policy.Task))
	defer cancel()
	leaseDone := make(chan struct{})
	leaseLost := make(chan error, 1)
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := s.repo.RenewTaskLease(task.ID, task.LeaseOwner, task.Attempts, 45*time.Second); err != nil {
					leaseLost <- err
					cancel()
					return
				}
			case <-leaseDone:
				return
			}
		}
	}()
	defer close(leaseDone)
	s.registerActiveTask(task.ID, cancel)
	defer s.unregisterActiveTask(task.ID)

	task.Stage = "调用生成模型"
	task.Progress = 35
	if err := s.repo.UpdateTaskProgress(task.ID, task.LeaseOwner, task.Attempts, task.Stage, task.Progress); err != nil {
		return fmt.Errorf("更新任务进度失败，任务暂未调用上游：%w", err)
	}
	if shutdownErr := workerShutdownCause(ctx); shutdownErr != nil {
		return shutdownErr
	}
	routeAttempt, err := s.beginTaskRouteAttempt(task)
	if err != nil {
		return terminal.markPreparationFailure(task, "路由准备失败", err, isRouteDispatchUncertain(err), "路由准备失败，上游请求未发出")
	}
	if shutdownErr := workerShutdownCause(ctx); shutdownErr != nil {
		return shutdownErr
	}
	if err := s.taskBilling().MarkBillingRunning(task.BillingOrderID); err != nil {
		return terminal.markPreparationFailure(task, "计费准备失败", err, false, "计费准备失败，上游请求未发出")
	}
	if shutdownErr := workerShutdownCause(ctx); shutdownErr != nil {
		return shutdownErr
	}
	routeResult, stateErr := s.routeExecutor().execute(ctx, task, routeAttempt)
	if shutdownErr := workerShutdownCause(ctx); shutdownErr != nil && (stateErr != nil || routeResult.err != nil) {
		// 路由执行器已把上游 ID/提交不确定态写回。关机只放弃当前租约，
		// 不能把进程生命周期取消投影成用户取消或失败退款。若上游已经
		// 成功返回结果，则继续完成本地持久化，避免同步结果在关机时丢失。
		return shutdownErr
	}
	if leaseErr := globalSlotLeaseCause(ctx); leaseErr != nil {
		return leaseErr
	}
	if stateErr != nil {
		return stateErr
	}
	result, canvasOps, err := routeResult.result, routeResult.canvasOps, routeResult.err
	providerSucceeded := routeResult.providerSucceeded
	if err == nil {
		// 上游调用可能跨越一次租约换手；落地媒体前确认仍持有本轮执行权，并给持久化留出完整租期。
		err = s.repo.RenewTaskLease(task.ID, task.LeaseOwner, task.Attempts, 45*time.Second)
	}
	if err == nil {
		result, err = s.persistGeneratedTaskMediaResult(task, result)
	}
	if leaseErr := globalSlotLeaseCause(ctx); leaseErr != nil {
		return leaseErr
	}
	if err == nil {
		err = s.repo.RenewTaskLease(task.ID, task.LeaseOwner, task.Attempts, 45*time.Second)
	}
	if err == nil {
		_, err = s.finalizeCharacterTurnaroundTask(*task, result)
	}
	if err != nil {
		channelSlotFailedBeforeRequest := false
		if code, _ := ChannelSlotFailureDetails(err); code != "" {
			channelSlotFailedBeforeRequest = true
		}
		select {
		case leaseErr := <-leaseLost:
			_ = s.log(task.UserID, task.ID, "warn", "任务租约失效，等待其他 worker 恢复", leaseErr.Error())
			return leaseErr
		default:
		}
		if errors.Is(err, context.DeadlineExceeded) {
			decryptedInput, decryptErr := s.decryptTaskInputJSON(task.InputJSON)
			if decryptErr == nil && s.shouldDeferProviderTask(*task, decryptedInput, err) {
				if deferErr := s.repo.DeferRunningTaskForProviderPoll(task.ID, task.LeaseOwner, task.Attempts, "后台仍在生成", 15*time.Second); deferErr != nil {
					return deferErr
				}
				_ = s.log(task.UserID, task.ID, "info", "本轮等待结束，上游任务仍在生成，将继续回查原任务", task.PollStage)
				return nil
			}
			err = errors.New(taskTimeoutMessage(task.Type))
		}
		return terminal.handleExecutionFailure(task, err, providerSucceeded, channelSlotFailedBeforeRequest)
	}
	latest, err := s.repo.Task(task.ID)
	if err != nil {
		return err
	}
	if latest.Status == model.TaskStatusCancelled {
		return terminal.handleCancelledResult(*latest)
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		_, terminalErr := terminal.handleResultPersistenceFailure(task, fmt.Errorf("序列化任务结果失败：%w", err))
		return terminalErr
	}
	opsJSON, err := json.Marshal(canvasOps)
	if err != nil {
		_, terminalErr := terminal.handleResultPersistenceFailure(task, fmt.Errorf("序列化画布操作失败：%w", err))
		return terminalErr
	}
	if err := s.saveTaskCompletionWithinStorageQuota(task, resultJSON, opsJSON, len(canvasOps) > 0); err != nil {
		_, terminalErr := terminal.handleResultPersistenceFailure(task, err)
		return terminalErr
	}
	return terminal.handleSuccess(task)
}

func globalSlotLeaseCause(ctx context.Context) error {
	if err := context.Cause(ctx); errors.Is(err, errRuntimeSlotLeaseLost) {
		return err
	}
	return nil
}

func taskFailureMessage(err error) string {
	if err == nil {
		return "任务处理失败"
	}
	return truncateRunes(err.Error(), 2_000)
}

func taskExecutionTimeoutWithPolicy(taskType string, policy RuntimeTaskPolicy) time.Duration {
	switch {
	case taskType == "agent_storyboard" || taskType == "agent_storyboard_rows":
		return time.Duration(policy.StoryboardTimeoutMinutes) * time.Minute
	case strings.HasPrefix(taskType, "canvas_video") || strings.HasPrefix(taskType, "video_"):
		return max(time.Duration(policy.VideoTimeoutMinutes)*time.Minute, 5*time.Minute)
	case strings.HasPrefix(taskType, "canvas_image"):
		return time.Duration(policy.ImageTimeoutMinutes) * time.Minute
	case strings.HasPrefix(taskType, "canvas_audio"):
		return time.Duration(policy.AudioTimeoutMinutes) * time.Minute
	case strings.HasPrefix(taskType, "canvas_text"):
		return time.Duration(policy.TextTimeoutMinutes) * time.Minute
	default:
		return time.Duration(policy.DefaultTimeoutMinutes) * time.Minute
	}
}

func (s *Service) shouldDeferProviderTask(task model.Task, decryptedInput string, err error) bool {
	if !errors.Is(err, context.DeadlineExceeded) || strings.TrimSpace(task.ProviderRequestID) == "" {
		return false
	}
	var input canvasGenerationInput
	if json.Unmarshal([]byte(decryptedInput), &input) != nil {
		return false
	}
	resolved, resolveErr := s.resolveProviderConfig(input.Config)
	if resolveErr != nil {
		return false
	}
	return providerPollResumeSupported(task.Type, resolved)
}

// providerPollResumeSupported 只列出已实现“带既有上游 ID 查询”的协议。
// 不能因为任务有 ProviderRequestID 就默认重试，否则未知渠道可能把 ID 当新请求重复提交。
func providerPollResumeSupported(taskType string, config providerConfig) bool {
	if strings.HasPrefix(taskType, "canvas_image") {
		return config.InterfaceType == string(model.ChannelInterfaceBailianImage)
	}
	if strings.HasPrefix(taskType, "canvas_audio") {
		return config.InterfaceType == string(model.ChannelInterfaceAsyncAudio)
	}
	if !strings.HasPrefix(taskType, "canvas_video") && !strings.HasPrefix(taskType, "video_") {
		return false
	}
	switch config.InterfaceType {
	case string(model.ChannelInterfaceNewAPIChannel1),
		string(model.ChannelInterfaceNewAPIChannel2),
		string(model.ChannelInterfaceAIAPIWayVideo),
		string(model.ChannelInterfaceVolcengineArkVideo),
		string(model.ChannelInterfaceVolcengineJiMengVideo),
		string(model.ChannelInterfaceGeminiVeo),
		string(model.ChannelInterfaceNovitaVideo),
		string(model.ChannelInterfaceMiniMaxVideo):
		return true
	default:
		return isArkPlanVideoConfig(config) || isSeedanceVideoConfig(config)
	}
}

func taskTimeoutMessage(taskType string) string {
	if strings.HasPrefix(taskType, "canvas_video") || strings.HasPrefix(taskType, "video_") {
		return "视频生成等待超时，请稍后到任务中心查看或重试。"
	}
	if strings.HasPrefix(taskType, "canvas_image") {
		return "图片生成等待超时，请稍后重试。"
	}
	return "任务执行超时，请稍后重试。"
}
