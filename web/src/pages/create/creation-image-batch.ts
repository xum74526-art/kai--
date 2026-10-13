import type { CreationImageBatch, CreationImageBatchItem, CreationMessage } from "./creation-types";

export function createCreationImageBatch(count: number): CreationImageBatch {
    if (!Number.isInteger(count) || count < 1 || count > 15) throw new Error("图片批次数量无效");
    return { count, items: Array.from({ length: count }, (_, batchIndex) => ({ batchIndex, status: "pending" })) };
}

export function creationImageBatchSummary(batch: CreationImageBatch) {
    const succeeded = batch.items.filter((item) => item.status === "succeeded").length;
    const failed = batch.items.filter((item) => item.status === "failed").length;
    const pending = batch.items.filter((item) => item.status === "pending").length;
    const unknown = batch.items.filter((item) => item.status === "unknown").length;
    const label = [
        succeeded ? `${succeeded} 张已生成` : "",
        failed ? `${failed} 张失败` : "",
        pending ? `${pending} 张处理中` : "",
        unknown ? `${unknown} 张状态待确认` : "",
    ].filter(Boolean).join("，");
    return { succeeded, failed, pending, unknown, label };
}

export function retryableCreationImageBatchItems(batch: CreationImageBatch): CreationImageBatchItem[] {
    // 未拿到任务标识的未知提交可能已经入库，不能把网络重试变成第二次计费。
    return batch.items.filter((item) => item.status === "failed" || (item.status === "unknown" && Boolean(item.taskId)));
}

export function restoreCreationImageBatchAfterReload(message: CreationMessage): CreationMessage {
    if (!message.imageBatch) return message;
    // 页面中断时未收到任务标识，不代表后端没有创建成功；刷新只能标记待核对。
    const items = message.imageBatch.items.map((item): CreationImageBatchItem => item.status === "pending" && !item.taskId ? {
        ...item,
        status: "unknown",
        failurePhase: "submission",
        error: "提交结果待确认，请先在任务中心核对结果，避免重复生成和计费。",
    } : item);
    return summarizeCreationImageBatch({ ...message, imageBatch: { ...message.imageBatch, items } });
}

export function summarizeCreationImageBatch(message: CreationMessage): CreationMessage {
    if (!message.imageBatch) return message;
    const summary = creationImageBatchSummary(message.imageBatch);
    const failure = message.imageBatch.items.find((item) => item.status === "failed" || item.status === "unknown");
    const hasResults = Boolean(message.resultStorageKeys?.length || message.resultUrls?.length || summary.succeeded);
    return {
        ...message,
        status: summary.pending ? "pending" : hasResults ? "done" : "error",
        content: summary.label,
        error: failure?.error,
        taskIds: Array.from(new Set([...(message.taskIds || []), ...message.imageBatch.items.flatMap((item) => item.taskId ? [item.taskId] : [])])),
    };
}

export function updateCreationImageBatchItem(message: CreationMessage, batchIndex: number, patch: Partial<Omit<CreationImageBatchItem, "batchIndex">>): CreationMessage {
    const batch = message.imageBatch;
    const current = batch?.items.find((item) => item.batchIndex === batchIndex);
    if (!batch || !current) throw new Error("图片批次缺少对应结果槽位");
    // 迟到的轮询不能把已挂载的成功图片改回失败或待生成。
    if (current.status === "succeeded" && patch.status && patch.status !== "succeeded") return message;
    return summarizeCreationImageBatch({
        ...message,
        imageBatch: { ...batch, items: batch.items.map((item) => item.batchIndex === batchIndex ? { ...item, ...patch } : item) },
    });
}
