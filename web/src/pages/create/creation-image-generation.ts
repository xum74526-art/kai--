import { generationErrorMessage } from "@/lib/generation-error";
import type { CreateTaskInput, GenerationTask } from "@/services/api/task-center";
import type { CreationImageBatchItem, CreationMessage } from "./creation-types";
import { updateCreationImageBatchItem } from "./creation-image-batch";

type Runtime = Pick<typeof import("./creation-runtime"),
    "prepareBackendGenerationTask" | "createGenerationTask" | "queryGenerationTask" | "retryGenerationTask" |
    "recoverGenerationTaskMedia" | "waitForGenerationTask" |
    "consumeGenerationTaskMessage" | "generationTaskMaterializedUrls" | "applyGenerationConsumerEffect">;

type ImageBatchExecution = {
    runtime: Runtime;
    request: Parameters<Runtime["prepareBackendGenerationTask"]>[0];
    count: number;
    items: CreationImageBatchItem[];
    messageId: string;
    updateMessage: (updater: (message: CreationMessage) => CreationMessage) => Promise<void>;
    onTaskUpdate: (task: GenerationTask) => void;
};

// 只有明确收到拒绝创建的响应，才允许补发无 taskId 的槽位；断网/超时可能已经提交成功。
function submissionRejected(error: unknown) {
    if (!error || typeof error !== "object" || !("status" in error)) return false;
    return [400, 401, 403, 404, 422, 429].includes(Number(error.status));
}

function failedItem(error: unknown, task: GenerationTask | undefined, phase: CreationImageBatchItem["failurePhase"]): Partial<CreationImageBatchItem> {
    const errorMessage = generationErrorMessage(error);
    if (task?.status === "succeeded" || phase === "materialization" || task?.canRecoverMedia || task?.mediaStage) {
        return { status: "failed", failurePhase: "materialization", error: errorMessage };
    }
    if (task?.status === "failed" || task?.status === "cancelled") {
        return { status: "failed", failurePhase: "generation", error: errorMessage };
    }
    if (!task && submissionRejected(error)) {
        return { status: "failed", failurePhase: "submission", error: errorMessage };
    }
    return {
        status: "unknown", failurePhase: task ? "generation" : "submission",
        error: `${errorMessage} ${task ? "任务状态待同步，请先恢复原任务。" : "提交结果待确认，请先核对任务中心，避免重复生成。"}`,
    };
}

export async function executeCreationImageBatch(options: ImageBatchExecution) {
    const { runtime, request, count, messageId, updateMessage } = options;
    // 重试始终显式列出失败槽位；成功槽位绝不再次调用上游。
    let selected = options.items.filter((item) => item.status !== "succeeded" && !(item.status === "unknown" && !item.taskId));
    if (!selected.length) return;
    for (const item of selected) {
        await updateMessage((message) => updateCreationImageBatchItem(message, item.batchIndex, { status: "pending", error: undefined }));
    }
    let prepared: CreateTaskInput | undefined;
    if (selected.some((item) => !item.taskId)) {
        try {
            // 同批参考素材只校验、上传一次，两个输出复用稳定资源 ID。
            prepared = await runtime.prepareBackendGenerationTask({ ...request, mode: "image", config: { ...request.config, count: "1" } });
        } catch (error) {
            for (const item of selected.filter((candidate) => !candidate.taskId)) {
                await updateMessage((message) => updateCreationImageBatchItem(message, item.batchIndex, {
                    status: "failed", failurePhase: "submission", error: generationErrorMessage(error),
                }));
            }
            selected = selected.filter((item) => Boolean(item.taskId));
        }
    }
    const executions = await Promise.allSettled(selected.map(async (item) => {
        let task: GenerationTask | undefined;
        let phase: CreationImageBatchItem["failurePhase"] = item.taskId ? item.failurePhase || "generation" : "submission";
        let persistenceError: unknown;
        let taskWrites = Promise.resolve();
        const onTaskUpdate = (next: GenerationTask) => {
            task = {
                ...next,
                clientContext: { ...next.clientContext, conversationId: String(request.metadata?.conversationId || ""), messageId, batchIndex: item.batchIndex, batchCount: count },
            };
            const snapshot = task;
            options.onTaskUpdate(snapshot);
            taskWrites = taskWrites.then(async () => {
                await updateMessage((message) => updateCreationImageBatchItem({
                    ...message,
                    taskIds: Array.from(new Set([...(message.taskIds || []), snapshot.id])),
                    generationStage: snapshot.stage,
                }, item.batchIndex, { taskId: snapshot.id, status: "pending" }));
            }).catch((error: unknown) => { persistenceError = error; });
        };
        try {
            if (request.signal?.aborted) throw new DOMException("Aborted", "AbortError");
            if (item.taskId) {
                task = await runtime.queryGenerationTask(item.taskId, { signal: request.signal });
                if (request.signal?.aborted) throw new DOMException("Aborted", "AbortError");
                if (task.status === "failed" || task.status === "cancelled") {
                    if (task.canRecoverMedia) {
                        phase = "materialization";
                        task = await runtime.recoverGenerationTaskMedia(task.id);
                    } else if (phase === "materialization" || task.mediaStage) {
                        phase = "materialization";
                        throw new Error("作品已生成，但结果暂时无法恢复；请在任务中心检查保存状态，不要重复生成");
                    } else {
                        task = await runtime.retryGenerationTask(task.id);
                    }
                }
                onTaskUpdate(task);
                if (task.status !== "succeeded") {
                    task = await runtime.waitForGenerationTask(task.id, { signal: request.signal, initialTask: task, onTaskUpdate });
                }
            } else {
                if (!prepared) throw new Error("图片任务准备未完成");
                task = await runtime.createGenerationTask({
                    ...prepared,
                    input: {
                        ...prepared.input,
                        metadata: { ...(prepared.input?.metadata as Record<string, unknown> | undefined), batchIndex: item.batchIndex, batchCount: count },
                    },
                });
                onTaskUpdate(task);
                task = await runtime.waitForGenerationTask(task.id, { signal: request.signal, initialTask: task, onTaskUpdate });
            }
            await taskWrites;
            if (persistenceError) throw persistenceError;
            phase = "materialization";
            // 后端成功但本地挂载失败，只重新读取既有产物，不创建新的收费任务。
            const materialized = await runtime.consumeGenerationTaskMessage(task, messageId, async ({ resultUrls, resultStorageKeys, effectKey }) => {
                await updateMessage((message) => runtime.applyGenerationConsumerEffect(message, effectKey, (current) => ({
                    ...current,
                    ...(resultUrls.length ? { resultUrls: Array.from(new Set([...(current.resultUrls || []), ...resultUrls])) } : {}),
                    ...(resultStorageKeys.length ? { resultStorageKeys: Array.from(new Set([...(current.resultStorageKeys || []), ...resultStorageKeys])) } : {}),
                })).value);
            }, { signal: request.signal });
            if (!runtime.generationTaskMaterializedUrls(materialized).length) throw new Error("图片已生成，但结果资源暂时不可用");
            await updateMessage((message) => updateCreationImageBatchItem(message, item.batchIndex, { taskId: task!.id, status: "succeeded", error: undefined, failurePhase: undefined }));
        } catch (error) {
            await taskWrites;
            await updateMessage((message) => updateCreationImageBatchItem(message, item.batchIndex, { ...failedItem(error, task, phase), ...(task ? { taskId: task.id } : {}) }));
            if (persistenceError) throw persistenceError;
        }
    }));
    // 保存失败必须向页面报告；不把未持久化的批次状态当成可靠结果。
    const rejected = executions.find((entry): entry is PromiseRejectedResult => entry.status === "rejected");
    if (rejected) throw rejected.reason;
}
