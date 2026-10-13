import { describe, expect, test } from "bun:test";
import { createCreationImageBatch, creationImageBatchSummary, restoreCreationImageBatchAfterReload, retryableCreationImageBatchItems, summarizeCreationImageBatch, updateCreationImageBatchItem } from "../src/pages/create/creation-image-batch";
import { attachCreationTaskContexts, reconcileCreationTaskMessages } from "../src/pages/create/creation-conversations";
import type { CreationConversation, CreationMessage } from "../src/pages/create/creation-types";
import { pendingCreationTaskIds, pendingCreationTaskKey } from "../src/services/creation-conversation-store";
import type { GenerationTask } from "../src/services/api/task-center";

function imageMessage(count = 2): CreationMessage {
    return { id: "message", role: "assistant", mode: "image", content: "", status: "pending", createdAt: "2026-01-01T00:00:00.000Z", imageBatch: createCreationImageBatch(count) };
}

function conversation(message: CreationMessage): CreationConversation {
    return { id: "conversation", title: "图片", updatedAt: message.createdAt || "2026-01-01T00:00:00.000Z", messages: [{ id: "prompt", role: "user", content: "参考画面", createdAt: message.createdAt }, message] };
}

function task(id: string, batchIndex: number, status: GenerationTask["status"]): GenerationTask {
    return {
        id, type: "canvas_image", status, prompt: "参考画面", attempts: 1,
        createdAt: "2026-01-01T00:00:00.000Z", updatedAt: "2026-01-01T00:01:00.000Z",
        clientContext: { conversationId: "conversation", messageId: "message", batchIndex, batchCount: 2 },
    };
}

const runtime = {} as Parameters<typeof reconcileCreationTaskMessages>[0];

describe("creation image batch", () => {
    test("提交失败仍保留原始数量和失败槽位，成功图片不受影响", () => {
        let message = imageMessage();
        message = updateCreationImageBatchItem(message, 0, { status: "succeeded", taskId: "done" });
        message = { ...message, resultStorageKeys: ["resource:done"], generationEffectKeys: ["effect:done"] };
        message = updateCreationImageBatchItem(message, 1, { status: "failed", failurePhase: "submission", error: "任务未创建" });

        expect(message.status).toBe("done");
        expect(message.content).toBe("1 张已生成，1 张失败");
        expect(message.imageBatch?.count).toBe(2);
        expect(message.taskIds).toEqual(["done"]);
        expect(message.resultStorageKeys).toEqual(["resource:done"]);
        expect(message.generationEffectKeys).toEqual(["effect:done"]);
        expect(retryableCreationImageBatchItems(message.imageBatch!).map((item) => item.batchIndex)).toEqual([1]);
    });

    test("重试失败槽位保持成功图片可见，迟到更新不回退成功状态", () => {
        let message: CreationMessage = { ...imageMessage(), resultStorageKeys: ["resource:done"] };
        message = updateCreationImageBatchItem(message, 0, { status: "succeeded", taskId: "done" });
        message = updateCreationImageBatchItem(message, 1, { status: "pending", taskId: "retry" });
        expect(message.status).toBe("pending");
        expect(message.resultStorageKeys).toEqual(["resource:done"]);
        expect(updateCreationImageBatchItem(message, 0, { status: "failed", error: "迟到错误" })).toBe(message);
    });

    test("未知提交没有任务标识时不能自动重新生成", () => {
        let message = imageMessage(3);
        message = updateCreationImageBatchItem(message, 0, { status: "unknown", error: "提交结果未知" });
        message = updateCreationImageBatchItem(message, 1, { status: "unknown", taskId: "known" });
        message = updateCreationImageBatchItem(message, 2, { status: "succeeded", taskId: "done" });
        expect(retryableCreationImageBatchItems(message.imageBatch!).map((item) => item.batchIndex)).toEqual([1]);
        expect(creationImageBatchSummary(message.imageBatch!)).toMatchObject({ succeeded: 1, unknown: 2, failed: 0, pending: 0 });
    });

    test("批次数量与槽位更新强校验", () => {
        expect(() => createCreationImageBatch(0)).toThrow();
        expect(() => createCreationImageBatch(1.5)).toThrow();
        expect(() => updateCreationImageBatchItem(imageMessage(), 3, { status: "failed" })).toThrow();
        const message = { ...imageMessage(), imageBatch: undefined };
        expect(summarizeCreationImageBatch(message)).toBe(message);
    });
});

describe("creation image batch recovery", () => {
    test("刷新后未返回任务标识的提交转为待确认，不无限加载或自动补发", () => {
        let message = imageMessage();
        message = updateCreationImageBatchItem(message, 0, { status: "succeeded", taskId: "done" });
        message = { ...message, resultStorageKeys: ["resource:done"] };
        const restored = restoreCreationImageBatchAfterReload(message);
        expect(restored.imageBatch?.items[0]).toBe(message.imageBatch?.items[0]);
        expect(restored.imageBatch?.items[1]).toMatchObject({ status: "unknown", failurePhase: "submission" });
        expect(restored.imageBatch?.items[1].error).toContain("任务中心");
        expect(restored.status).toBe("done");
        expect(restored.resultStorageKeys).toEqual(["resource:done"]);
        expect(retryableCreationImageBatchItems(restored.imageBatch!)).toEqual([]);
        expect(pendingCreationTaskIds([conversation(restored)])).toEqual([]);
    });

    test("刷新只改变缺少任务标识的 pending，其他槽位仍按原状态恢复", () => {
        let message = imageMessage(3);
        message = updateCreationImageBatchItem(message, 0, { status: "pending", taskId: "known" });
        message = updateCreationImageBatchItem(message, 1, { status: "failed", failurePhase: "submission", error: "明确拒绝" });
        message = updateCreationImageBatchItem(message, 2, { status: "unknown", error: "等待人工核对" });
        const restored = restoreCreationImageBatchAfterReload(message);
        expect(restored.imageBatch?.items).toEqual(message.imageBatch?.items);
        expect(restored.status).toBe("pending");
        expect(pendingCreationTaskIds([conversation(restored)])).toEqual(["known"]);
        const withoutBatch = { ...message, imageBatch: undefined };
        expect(restoreCreationImageBatchAfterReload(withoutBatch)).toBe(withoutBatch);
    });

    test("逐个恢复乱序任务，不等待整个批次同时出现", () => {
        const message = updateCreationImageBatchItem(imageMessage(), 0, { status: "failed", failurePhase: "submission", error: "任务未创建" });
        const result = { ...task("second", 1, "succeeded"), creationResultStorageKeys: ["resource:second"] };
        const recovered = reconcileCreationTaskMessages(runtime, [conversation(message)], [result]);
        const assistant = recovered[0].messages[1];
        expect(assistant.imageBatch?.items[0]).toMatchObject({ batchIndex: 0, status: "failed", failurePhase: "submission" });
        expect(assistant.imageBatch?.items[1]).toMatchObject({ batchIndex: 1, status: "succeeded", taskId: "second" });
        expect(assistant.resultStorageKeys).toEqual(["resource:second"]);
        expect(assistant.content).toBe("1 张已生成，1 张失败");
        expect(reconcileCreationTaskMessages(runtime, recovered, [result])).toBe(recovered);
    });

    test("部分结果先挂载为 done 后仍恢复其他槽位的失败", () => {
        let message = updateCreationImageBatchItem(imageMessage(), 0, { status: "succeeded", taskId: "first" });
        message = { ...message, status: "done", resultStorageKeys: ["resource:first"] };
        const failed = { ...task("second", 1, "failed"), error: "供应商暂时不可用" };
        const recovered = reconcileCreationTaskMessages(runtime, [conversation(message)], [failed])[0].messages[1];
        expect(recovered.resultStorageKeys).toEqual(["resource:first"]);
        expect(recovered.content).toBe("1 张已生成，1 张失败");
        expect(recovered.imageBatch?.items[1].failurePhase).toBe("generation");
    });

    test("后端生成成功但前端资源化失败只标为保存失败", () => {
        const completed = { ...task("first", 0, "succeeded"), creationError: "资源缓存暂时不可用" };
        const recovered = reconcileCreationTaskMessages(runtime, [conversation(imageMessage())], [completed])[0].messages[1];
        expect(recovered.imageBatch?.items[0]).toMatchObject({ taskId: "first", status: "failed", failurePhase: "materialization" });
        const saveFailed = { ...task("second", 1, "failed"), mediaStage: "upload" as const, canRecoverMedia: true, error: "保存失败" };
        const completedBatch = reconcileCreationTaskMessages(runtime, [conversation(recovered)], [saveFailed])[0].messages[1];
        expect(completedBatch.imageBatch?.items[1].failurePhase).toBe("materialization");
    });

    test("taskIds 返回顺序不能覆盖真实批次索引", () => {
        const message = { ...imageMessage(), taskIds: ["second", "first"] };
        const tasks = attachCreationTaskContexts([task("second", 1, "queued"), task("first", 0, "queued")], [conversation(message)]);
        expect(tasks.map((entry) => entry.clientContext?.batchIndex)).toEqual([1, 0]);
        expect(tasks.map((entry) => entry.clientContext?.batchCount)).toEqual([2, 2]);
    });

    test("恢复订阅只跟踪进行中和带任务标识的未知槽位", () => {
        let message = imageMessage(4);
        message = updateCreationImageBatchItem(message, 0, { status: "succeeded", taskId: "done" });
        message = updateCreationImageBatchItem(message, 1, { status: "pending", taskId: "pending" });
        message = updateCreationImageBatchItem(message, 2, { status: "unknown", taskId: "unknown" });
        message = updateCreationImageBatchItem(message, 3, { status: "unknown" });
        const conversations = [conversation({ ...message, status: "done" })];
        expect(pendingCreationTaskIds(conversations)).toEqual(["pending", "unknown"]);
        expect(pendingCreationTaskKey(conversations)).toBe("conversation:message:pending,unknown");
    });
});
