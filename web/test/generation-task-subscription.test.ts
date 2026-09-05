import { expect, test } from "bun:test";

import { ApiError } from "../src/services/api/request";
import { createGenerationTaskSubscriptionService, type GenerationTask, type GenerationTaskSubscriptionError } from "../src/services/api/task-center";

const completedTask: GenerationTask = {
    id: "task-subscription-retry",
    type: "canvas_image",
    status: "succeeded",
    prompt: "retry observation",
    attempts: 1,
    createdAt: "2026-09-02T00:00:00.000Z",
    updatedAt: "2026-09-02T00:00:01.000Z",
};

test("task subscription retries after a transient initial query failure", async () => {
    let queryCalls = 0;
    const service = createGenerationTaskSubscriptionService({
        async queryTask() {
            queryCalls += 1;
            if (queryCalls === 1) throw new Error("temporary network failure");
            return completedTask;
        },
        async waitTask() {
            throw new Error("terminal task must not be polled");
        },
    });
    const received: GenerationTask[] = [];
    const unsubscribe = service.subscribe([completedTask.id], (task) => received.push(task));

    await new Promise((resolve) => setTimeout(resolve, 1_100));
    unsubscribe();

    expect(queryCalls).toBe(2);
    expect(received).toEqual([completedTask]);
});

test("task subscription stops immediately for unrecoverable 4xx API errors and reports them", async () => {
    for (const status of [401, 403, 404]) {
        let queryCalls = 0;
        const errors: GenerationTaskSubscriptionError[] = [];
        const service = createGenerationTaskSubscriptionService({
            async queryTask() {
                queryCalls += 1;
                throw new ApiError("任务不可读取", { status });
            },
            async waitTask() {
                throw new Error("unreachable");
            },
            retryDelayMs: () => 0,
        });
        const taskId = `task-${status}`;
        const unsubscribe = service.subscribe([taskId], () => {}, { onError: (event) => errors.push(event) });

        await new Promise((resolve) => setTimeout(resolve, 0));
        unsubscribe();

        expect(queryCalls).toBe(1);
        expect(errors).toHaveLength(1);
        expect(errors[0]).toMatchObject({ taskId, retryable: false, retryCount: 0, willRetry: false });
    }
});

test("task subscription stops after its bounded transient retry budget", async () => {
    let queryCalls = 0;
    const errors: GenerationTaskSubscriptionError[] = [];
    const service = createGenerationTaskSubscriptionService({
        async queryTask() {
            queryCalls += 1;
            throw new Error("temporary network failure");
        },
        async waitTask() {
            throw new Error("unreachable");
        },
        maxRetries: 2,
        retryDelayMs: () => 0,
    });
    const unsubscribe = service.subscribe(["task-transient"], () => {}, { onError: (event) => errors.push(event) });

    await new Promise((resolve) => setTimeout(resolve, 30));
    unsubscribe();

    expect(queryCalls).toBe(3);
    expect(errors.map((event) => event.willRetry)).toEqual([true, true, false]);
    expect(errors.at(-1)).toMatchObject({ taskId: "task-transient", retryable: true, retryCount: 2, willRetry: false });
});

test("task subscription does not reset its retry budget after a successful initial query", async () => {
    let queryCalls = 0;
    let waitCalls = 0;
    const errors: GenerationTaskSubscriptionError[] = [];
    const runningTask: GenerationTask = { ...completedTask, id: "task-wait-transient", status: "running" };
    const service = createGenerationTaskSubscriptionService({
        async queryTask() {
            queryCalls += 1;
            return runningTask;
        },
        async waitTask() {
            waitCalls += 1;
            throw new Error("temporary network failure");
        },
        maxRetries: 2,
        retryDelayMs: () => 0,
    });
    const unsubscribe = service.subscribe([runningTask.id], () => {}, { onError: (event) => errors.push(event) });

    await new Promise((resolve) => setTimeout(resolve, 30));
    unsubscribe();

    expect(queryCalls).toBe(3);
    expect(waitCalls).toBe(3);
    expect(errors.map((event) => event.willRetry)).toEqual([true, true, false]);
});

test("task subscription does not report a terminal failure update as an observation failure", async () => {
    const runningTask: GenerationTask = { ...completedTask, id: "task-terminal-failure", status: "running" };
    const failedTask: GenerationTask = { ...runningTask, status: "failed", error: "模型拒绝请求" };
    const received: GenerationTask[] = [];
    const errors: GenerationTaskSubscriptionError[] = [];
    const service = createGenerationTaskSubscriptionService({
        async queryTask() {
            return runningTask;
        },
        async waitTask(_id, options) {
            options?.onTaskUpdate?.(failedTask);
            throw new Error("模型拒绝请求");
        },
        retryDelayMs: () => 0,
    });

    const unsubscribe = service.subscribe([runningTask.id], (task) => received.push(task), { onError: (event) => errors.push(event) });
    await new Promise((resolve) => setTimeout(resolve, 0));
    unsubscribe();

    expect(received).toEqual([runningTask, failedTask]);
    expect(errors).toEqual([]);
});

test("Create handles final subscription failures instead of retaining a pending message", async () => {
    const source = await Bun.file(new URL("../src/pages/create/index.tsx", import.meta.url)).text();
    const taskCenterSource = await Bun.file(new URL("../src/services/api/task-center.ts", import.meta.url)).text();

    expect(source).toContain("failCreationTaskObservation");
    expect(source).toContain("creationTaskObservationErrorMessage");
    expect(source).toContain("{ onError: handleSubscriptionError }");
    expect(source).toMatch(/const next = failCreationTaskObservation\(current, taskId, message\);[\s\S]{0,240}conversationsRef\.current = next;/);
    expect(source).toContain('status: "error" as const');
    expect(taskCenterSource).toContain("if (!isRetryableTaskObservationError(error)) throw error;");
    expect(taskCenterSource).toContain("consecutiveRetryableQueryFailures > MAX_TASK_OBSERVATION_RETRIES");
});
