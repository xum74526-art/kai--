import { expect, test } from "bun:test";

import { createGenerationTaskSubscriptionService, type GenerationTask } from "../src/services/api/task-center";

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
