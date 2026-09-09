import { generationErrorMessage } from "@/lib/generation-error";
import type { GenerationTask } from "@/services/api/task-center";
import type { CreationAttachment } from "./creation-assets";
import type { CreationReference } from "./creation-references";

export type CreationMode = "text" | "image" | "video";
export type CreationStatus = "streaming" | "pending" | "done" | "error" | "cancelled";
export type CreationSettings = { ratio: string; seconds: string; quality: string; videoQuality: string; count: string };
export type CreationTaskTerminalStatus = Extract<GenerationTask["status"], "succeeded" | "failed" | "cancelled">;
export type CreationMessage = {
    id: string;
    role: "user" | "assistant";
    mode?: CreationMode;
    content: string;
    reasoning?: string;
    createdAt: string;
    status?: CreationStatus;
    model?: string;
    resultUrls?: string[];
    error?: string;
    generationErrorCode?: string;
    generationOperation?: string;
    attachments?: CreationAttachment[];
    references?: CreationReference[];
    settings?: CreationSettings;
    taskIds?: string[];
    // 批量生成的每项终态需要随对话一起保存，避免页面恢复时一项失败把其余任务的结果丢失。
    expectedTaskCount?: number;
    taskTerminalStates?: Record<string, CreationTaskTerminalStatus>;
    taskObservationErrors?: Record<string, string>;
    // 刷新后尚未找回任务时不把它误判为生成失败；用户可显式重新同步，不会重复提交。
    taskRecoveryError?: string;
    clientOperationId?: string;
    retryOf?: string;
    attemptGroupId?: string;
    generationStage?: string;
    generationEffectKeys?: string[];
};
export type CreationConversation = { id: string; title: string; updatedAt: string; messages: CreationMessage[] };

export function creationTaskObservationErrorMessage(error: unknown) {
    const candidate = error && typeof error === "object" ? (error as { status?: unknown; code?: unknown }) : undefined;
    const status = typeof candidate?.status === "number" ? candidate.status : typeof candidate?.code === "number" ? candidate.code : undefined;
    if (status === 401 || status === 403) return "任务状态同步失败：登录状态或访问权限已失效，请重新登录后重试。";
    if (status === 404) return "任务状态同步失败：任务不存在或已被删除。";
    const detail = generationErrorMessage(error).trim();
    return detail && detail !== "生成失败，请稍后重试。" ? `任务状态同步失败：${detail}` : "任务状态无法继续同步，请刷新后重新生成。";
}

export function failCreationTaskObservation(conversations: CreationConversation[], taskId: string, error: string): CreationConversation[] {
    let changed = false;
    const updatedAt = new Date().toISOString();
    const next = conversations.map((conversation) => {
        let conversationChanged = false;
        const messages = conversation.messages.map((message): CreationMessage => {
            const isPending = message.status === "pending" || (message.mode === "text" && message.status === "streaming");
            if (message.role !== "assistant" || !isPending || !message.taskIds?.includes(taskId)) return message;
            changed = true;
            conversationChanged = true;
            if (message.mode === "text") {
                return {
                    ...message,
                    status: "error",
                    content: message.content || "任务状态同步失败",
                    error,
                };
            }
            return applyCreationTaskOutcomes(message, {
                terminalStates: { [taskId]: "failed" },
                observationErrors: { [taskId]: error },
            });
        });
        return conversationChanged ? { ...conversation, messages, updatedAt } : conversation;
    });
    return changed ? next : conversations;
}

export function isCreationTaskTerminalStatus(status: GenerationTask["status"]): status is CreationTaskTerminalStatus {
    return status === "succeeded" || status === "failed" || status === "cancelled";
}

export function creationExpectedTaskCount(message: CreationMessage, additionalCount = 0) {
    const savedCount = Number.isSafeInteger(message.expectedTaskCount) ? Math.max(0, message.expectedTaskCount || 0) : 0;
    return Math.max(savedCount, message.taskIds?.length || 0, additionalCount);
}

type CreationTaskOutcomeInput = {
    taskIds?: string[];
    expectedTaskCount?: number;
    terminalStates?: Record<string, CreationTaskTerminalStatus>;
    observationErrors?: Record<string, string>;
    resultUrls?: string[];
    cancelledMessage?: string;
};

// 批量任务可能跨越页面刷新和多个订阅回调。把每个已终态的任务记入消息，
// 只有整批均结束后才落为 done/error，避免一项失败覆盖其他项的成功结果。
export function applyCreationTaskOutcomes(message: CreationMessage, input: CreationTaskOutcomeInput): CreationMessage {
    const taskIds = Array.from(new Set([...(message.taskIds || []), ...(input.taskIds || [])]));
    const expectedTaskCount = creationExpectedTaskCount(message, input.expectedTaskCount || 0);
    const terminalStates = { ...(message.taskTerminalStates || {}), ...(input.terminalStates || {}) };
    const observationErrors = { ...(message.taskObservationErrors || {}), ...(input.observationErrors || {}) };
    for (const taskId of taskIds) {
        if (observationErrors[taskId] && !terminalStates[taskId]) terminalStates[taskId] = "failed";
    }
    const resultUrls = Array.from(new Set([...(message.resultUrls || []), ...(input.resultUrls || [])]));
    const fullyTracked = taskIds.length >= expectedTaskCount;
    const allTerminal = fullyTracked && expectedTaskCount > 0 && taskIds.every((taskId) => Boolean(terminalStates[taskId]));
    const common = {
        ...message,
        taskIds,
        taskRecoveryError: undefined,
        ...(expectedTaskCount ? { expectedTaskCount } : {}),
        ...(resultUrls.length ? { resultUrls } : {}),
    };

    if (!allTerminal) {
        return {
            ...common,
            status: "pending",
            error: undefined,
            ...(Object.keys(terminalStates).length ? { taskTerminalStates: terminalStates } : {}),
            ...(Object.keys(observationErrors).length ? { taskObservationErrors: observationErrors } : {}),
        };
    }

    const statuses = taskIds.map((taskId) => terminalStates[taskId]);
    if (resultUrls.length) {
        const incompleteCount = statuses.filter((status) => status !== "succeeded").length;
        const content = message.mode === "video" ? "视频已生成" : incompleteCount ? `${resultUrls.length} 张图片已生成，${incompleteCount} 项未完成` : "图片已生成";
        return { ...common, status: "done", content, error: undefined, taskTerminalStates: undefined, taskObservationErrors: undefined };
    }
    if (statuses.every((status) => status === "cancelled")) {
        return { ...common, status: "cancelled", content: input.cancelledMessage || "已停止", error: undefined, taskTerminalStates: undefined, taskObservationErrors: undefined };
    }
    const failure = taskIds.map((taskId) => observationErrors[taskId]).find(Boolean) || "任务已结束，但生成结果暂时无法读取";
    return { ...common, status: "error", content: "生成失败", error: failure, taskTerminalStates: undefined, taskObservationErrors: undefined };
}
