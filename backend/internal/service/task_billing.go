package service

import (
	"errors"
	"fmt"
	"strings"

	"infinite-canvas/backend/internal/model"
)

// taskBillingCoordinator 只负责任务生命周期中的计费状态迁移和核对策略。
//
// 兑换码、充值和管理员人工核对仍属于 finance service；这里收敛的是任务
// 从排队、上游调用到终态这一条链路，避免 worker、provider 和终态协调器
// 各自决定如何退款或进入待核对。
type taskBillingCoordinator struct {
	repo taskBillingRepository
}

type taskBillingRepository interface {
	MarkBillingRunning(orderID string) error
	SettleBillingOrder(orderID string, providerRequestID string) error
	RefundBillingOrder(orderID string, errorText string) error
	MarkBillingUncertain(orderID string, errorText string) error
	BillingOrder(orderID string) (*model.BillingOrder, error)
	BillingOrderHasSuccessfulBillableCall(orderID string) (bool, error)
}

var (
	// errTaskBillingReview 表示订单状态已经不能由重试流程安全推断，必须先完成费用核对。
	errTaskBillingReview = errors.New("task billing order requires review")
)

func newTaskBillingCoordinator(repo taskBillingRepository) *taskBillingCoordinator {
	return &taskBillingCoordinator{repo: repo}
}

func (c *taskBillingCoordinator) MarkBillingRunning(orderID string) error {
	if orderID == "" {
		return nil
	}
	return c.repo.MarkBillingRunning(orderID)
}

func (c *taskBillingCoordinator) SettleBilling(orderID string, providerRequestID string) error {
	if orderID == "" {
		return nil
	}
	return c.repo.SettleBillingOrder(orderID, providerRequestID)
}

func (c *taskBillingCoordinator) RefundBilling(orderID string, errorText string) error {
	if orderID == "" {
		return nil
	}
	return c.repo.RefundBillingOrder(orderID, truncateRunes(errorText, 1000))
}

func (c *taskBillingCoordinator) MarkBillingUncertain(orderID string, errorText string) error {
	if orderID == "" {
		return nil
	}
	return c.repo.MarkBillingUncertain(orderID, truncateRunes(errorText, 1000))
}

// CheckRetryEligibility 收敛失败/取消任务重试前的计费状态校验。
//
// 计费订单读取失败不能被当作“没有待核对订单”继续放行，否则重试可能重复扣费。
// 只有明确读到订单且旧订单已经进入 settled/refunded 终态，才允许重新计费。
func (c *taskBillingCoordinator) CheckRetryEligibility(orderID string) error {
	if strings.TrimSpace(orderID) == "" {
		return nil
	}
	order, err := c.repo.BillingOrder(orderID)
	if err != nil {
		return fmt.Errorf("读取任务计费订单失败：%w", err)
	}
	if order == nil {
		return fmt.Errorf("任务计费订单不存在：%s", orderID)
	}
	if order.Status != model.BillingStatusSettled && order.Status != model.BillingStatusRefunded {
		return errTaskBillingReview
	}
	return nil
}

func (c *taskBillingCoordinator) BillingFailureRequiresReview(orderID string, _ string, err error) bool {
	if orderID == "" {
		return false
	}
	if billingFailureUncertain(err) {
		return true
	}
	order, orderErr := c.repo.BillingOrder(orderID)
	if orderErr != nil || order == nil || order.Status == model.BillingStatusUncertain {
		return true
	}
	// 重试会复用 Task ID，但每轮都有独立 BillingOrder；只检查本轮订单，避免旧调用证据污染新账单。
	hasSuccessfulCall, logErr := c.repo.BillingOrderHasSuccessfulBillableCall(orderID)
	return logErr != nil || hasSuccessfulCall
}

func billingFailureUncertain(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, marker := range []string{"524", "timeout", "超时", "deadline exceeded", "context canceled", "connection reset", "unexpected eof", "broken pipe"} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}
