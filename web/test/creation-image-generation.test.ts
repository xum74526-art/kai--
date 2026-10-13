import { expect, test } from "bun:test";
import { createCreationImageBatch } from "../src/pages/create/creation-image-batch";
import { executeCreationImageBatch } from "../src/pages/create/creation-image-generation";
import type { CreationImageBatchItem, CreationMessage } from "../src/pages/create/creation-types";
import type { CreateTaskInput, GenerationTask } from "../src/services/api/task-center";

type Execution = Parameters<typeof executeCreationImageBatch>[0];

function task(id: string, status: GenerationTask["status"] = "succeeded"): GenerationTask {
    return { id, status, type: "canvas_image", prompt: "test", attempts: 1, createdAt: "2026-01-01", updatedAt: "2026-01-01" };
}

function fixture(items?: CreationImageBatchItem[]) {
    let message: CreationMessage = {
        id: "message", role: "assistant", mode: "image", content: "", status: "pending",
        imageBatch: items ? { count: items.length, items } : createCreationImageBatch(2),
    };
    const calls = { prepared: 0, created: [] as number[], queried: [] as string[], retried: [] as string[], recovered: [] as string[], consumed: [] as string[], waited: [] as string[] };
    const runtime = {
        async prepareBackendGenerationTask() {
            calls.prepared++;
            return { type: "canvas_image", prompt: "test", model: "test-model", input: { metadata: { conversationId: "conversation", messageId: "message" } } };
        },
        async createGenerationTask(input: CreateTaskInput) {
            const metadata = input.input?.metadata as { batchIndex: number };
            calls.created.push(metadata.batchIndex);
            return task(`task-${metadata.batchIndex}`, "queued");
        },
        async queryGenerationTask(id: string) {
            calls.queried.push(id);
            return task(id);
        },
        async retryGenerationTask(id: string) {
            calls.retried.push(id);
            return task(id, "queued");
        },
        async recoverGenerationTaskMedia(id: string) {
            calls.recovered.push(id);
            return task(id, "queued");
        },
        async waitForGenerationTask(id: string, options: { onTaskUpdate?: (value: GenerationTask) => void }) {
            calls.waited.push(id);
            const completed = task(id);
            options.onTaskUpdate?.(completed);
            return completed;
        },
        async consumeGenerationTaskMessage(value: GenerationTask, _messageId: string, consume: (value: { resultUrls: string[]; resultStorageKeys: string[]; effectKey: string }) => Promise<void>) {
            calls.consumed.push(value.id);
            await consume({ resultUrls: [`image:${value.id}`], resultStorageKeys: [`resource:${value.id}`], effectKey: `effect:${value.id}` });
            return value;
        },
        generationTaskMaterializedUrls(value: GenerationTask) { return [`image:${value.id}`]; },
        applyGenerationConsumerEffect(value: CreationMessage, key: string, update: (current: CreationMessage) => CreationMessage) {
            return { value: { ...update(value), generationEffectKeys: [...(value.generationEffectKeys || []), key] } };
        },
    };
    return {
        calls, runtime,
        get message() { return message; },
        set message(value: CreationMessage) { message = value; },
        async run() {
            await executeCreationImageBatch({
                runtime: runtime as unknown as Execution["runtime"],
                request: { mode: "image", prompt: "test", config: {} as Execution["request"]["config"], metadata: { conversationId: "conversation" } },
                count: message.imageBatch!.count, items: message.imageBatch!.items, messageId: message.id,
                onTaskUpdate() {},
                async updateMessage(update) { message = update(message); },
            });
        },
    };
}

test("one rejected admission remains visible while its sibling succeeds", async () => {
    const run = fixture();
    const create = run.runtime.createGenerationTask;
    run.runtime.createGenerationTask = async (input) => {
        if ((input.input?.metadata as { batchIndex: number }).batchIndex === 0) {
            throw Object.assign(new Error("database is locked"), { status: 400 });
        }
        return create(input);
    };
    await run.run();
    expect(run.calls.prepared).toBe(1);
    expect(run.message.imageBatch?.items).toMatchObject([
        { batchIndex: 0, status: "failed", failurePhase: "submission" },
        { batchIndex: 1, status: "succeeded", taskId: "task-1" },
    ]);
    expect(run.message.resultStorageKeys).toEqual(["resource:task-1"]);
    expect(run.message.content).toContain("1 张失败");
});

test("retry submits only the failed slot and preserves existing successful media", async () => {
    const run = fixture([{ batchIndex: 0, status: "failed", failurePhase: "submission" }, { batchIndex: 1, taskId: "saved", status: "succeeded" }]);
    run.message = { ...run.message, resultStorageKeys: ["resource:saved"], resultUrls: ["image:saved"], generationEffectKeys: ["effect:saved"] };
    await run.run();
    expect(run.calls.created).toEqual([0]);
    expect(run.calls.queried).toEqual([]);
    expect(run.calls.prepared).toBe(1);
    expect(run.message.resultStorageKeys).toEqual(["resource:saved", "resource:task-0"]);
    expect(run.message.generationEffectKeys).toContain("effect:saved");
});

test("successful upstream task with failed materialization never regenerates", async () => {
    const run = fixture([{ batchIndex: 0, taskId: "saved", status: "failed", failurePhase: "materialization" }]);
    await run.run();
    expect(run.calls.created).toEqual([]);
    expect(run.calls.retried).toEqual([]);
    expect(run.calls.prepared).toBe(0);
    expect(run.calls.consumed).toEqual(["saved"]);
    expect(run.message.imageBatch?.items[0].status).toBe("succeeded");
});

test("running task is observed, not submitted a second time", async () => {
    const run = fixture([{ batchIndex: 0, taskId: "running", status: "unknown", failurePhase: "generation" }]);
    run.runtime.queryGenerationTask = async (id) => task(id, "running");
    await run.run();
    expect(run.calls.created).toEqual([]);
    expect(run.calls.retried).toEqual([]);
    expect(run.calls.waited).toEqual(["running"]);
});

test("failed saved task uses backend retry while media failure uses recovery", async () => {
    const run = fixture([
        { batchIndex: 0, taskId: "generation", status: "failed", failurePhase: "generation" },
        { batchIndex: 1, taskId: "media", status: "failed", failurePhase: "materialization" },
    ]);
    run.runtime.queryGenerationTask = async (id) => ({ ...task(id, "failed"), canRecoverMedia: id === "media" });
    await run.run();
    expect(run.calls.created).toEqual([]);
    expect(run.calls.retried).toEqual(["generation"]);
    expect(run.calls.recovered).toEqual(["media"]);
});

test("unknown submission without a task id cannot be replayed", async () => {
    const run = fixture([{ batchIndex: 0, status: "unknown", failurePhase: "submission" }]);
    await run.run();
    expect(run.calls.created).toEqual([]);
    expect(run.calls.prepared).toBe(0);
});

test("lost creation response records uncertainty rather than a safe retry", async () => {
    const run = fixture([{ batchIndex: 0, status: "pending" }]);
    run.runtime.createGenerationTask = async () => { throw new Error("Network Error"); };
    await run.run();
    expect(run.message.imageBatch?.items[0]).toMatchObject({ status: "unknown", failurePhase: "submission" });
    expect(run.message.imageBatch?.items[0].taskId).toBeUndefined();
});

test("local materialization failure keeps the task id for save-only recovery", async () => {
    const run = fixture([{ batchIndex: 0, status: "pending" }]);
    run.runtime.consumeGenerationTaskMessage = async () => { throw new Error("素材保存失败"); };
    await run.run();
    expect(run.message.imageBatch?.items[0]).toMatchObject({ status: "failed", failurePhase: "materialization", taskId: "task-0" });
    expect(run.calls.created).toEqual([0]);
});

test("incomplete media checkpoint never falls back to a new upstream generation", async () => {
    const run = fixture([{ batchIndex: 0, taskId: "download-failed", status: "failed", failurePhase: "generation" }]);
    run.runtime.queryGenerationTask = async (id) => ({ ...task(id, "failed"), mediaStage: "download", canRecoverMedia: false });
    await run.run();
    expect(run.calls.created).toEqual([]);
    expect(run.calls.retried).toEqual([]);
    expect(run.message.imageBatch?.items[0]).toMatchObject({ status: "failed", failurePhase: "materialization" });
});
