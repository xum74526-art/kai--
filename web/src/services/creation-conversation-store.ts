import { localForageStorageForScope } from "@/lib/localforage-storage";
import { getActiveUserScope } from "@/lib/user-scope";

export const CREATION_CONVERSATIONS_KEY = "creation-conversations-v1";

type PendingCreationMessage = {
    id: string;
    role: "user" | "assistant";
    mode?: string;
    status?: string;
    taskIds?: string[];
    expectedTaskCount?: number;
    attachments?: Array<{ storageKey?: string; [key: string]: unknown }>;
};

export type StoredCreationConversation = {
    id: string;
    messages: PendingCreationMessage[];
};

export function updateCreationConversationSnapshot<T extends { id: string }>(conversations: T[], conversationId: string, updater: (conversation: T) => T) {
    return conversations.map((conversation) => (conversation.id === conversationId ? updater(conversation) : conversation));
}

// 对话、生成任务与素材是独立持久状态；删除历史记录不能在这里级联清理任务或资源。
export function removeCreationConversationSnapshot<T extends { id: string }>(conversations: T[], conversationId: string) {
    if (!conversationId) throw new Error("缺少要删除的创作对话 ID");
    const next = conversations.filter((conversation) => conversation.id !== conversationId);
    if (next.length === conversations.length) throw new Error("要删除的创作对话不存在");
    return next;
}

function expectedCreationTaskCount(message: PendingCreationMessage) {
    return Number.isSafeInteger(message.expectedTaskCount) ? Math.max(0, message.expectedTaskCount || 0) : 0;
}

function isRecoverableCreationMessage(message: PendingCreationMessage) {
    if (message.role !== "assistant") return false;
    const hasKnownTask = Boolean(message.taskIds?.length);
    if (message.mode === "text") return hasKnownTask && (message.status === "streaming" || message.status === "pending");
    return message.status === "pending" && (hasKnownTask || expectedCreationTaskCount(message) > 0);
}

export function pendingCreationTaskKey(conversations: StoredCreationConversation[]) {
    return conversations.flatMap((conversation) => conversation.messages.flatMap((message) => (isRecoverableCreationMessage(message) ? [`${conversation.id}:${message.id}:${(message.taskIds || []).join(",")}:${expectedCreationTaskCount(message)}`] : []))).join("|");
}

export function pendingCreationTaskIds(conversations: StoredCreationConversation[]) {
    const taskIds = conversations.flatMap((conversation) =>
        conversation.messages.flatMap((message) => {
            if (!isRecoverableCreationMessage(message)) return [];
            return message.taskIds || [];
        }),
    );
    return Array.from(new Set(taskIds));
}

// 批量任务的任务 ID 可能在浏览器刷新前尚未全部写入本地缓存。
// 仅为这类尚未完整追踪的媒体消息返回服务端关联键，供创作页按 clientContext 找回同一批任务。
export function pendingCreationTaskContextKeys(conversations: StoredCreationConversation[]) {
    return conversations.flatMap((conversation) => conversation.messages.flatMap((message) => {
        if (!isRecoverableCreationMessage(message) || message.mode === "text") return [];
        const expectedCount = expectedCreationTaskCount(message);
        const knownCount = message.taskIds?.length || 0;
        // 旧会话可能只保存了已知任务 ID、没有批量总数；这些任务由普通订阅恢复，
        // 不需要再触发按上下文的远端查询。只有明确还缺任务时才查询完整批次。
        if (expectedCount <= knownCount) return [];
        return [`${conversation.id}:${message.id}`];
    }));
}

export async function loadCreationConversations<T extends StoredCreationConversation>(scope = getActiveUserScope()) {
    const storage = localForageStorageForScope(scope);
    const value = await storage.getItem(CREATION_CONVERSATIONS_KEY);
    if (!value) return null;
    let parsed: unknown;
    try {
        parsed = JSON.parse(value);
    } catch {
        throw new Error("创作对话持久状态无效");
    }
    if (!Array.isArray(parsed)) throw new Error("创作对话持久状态无效");
    return parsed as T[];
}

export async function saveCreationConversations<T extends StoredCreationConversation>(conversations: T[]) {
    const storage = localForageStorageForScope(getActiveUserScope());
    await storage.setItem(CREATION_CONVERSATIONS_KEY, JSON.stringify(conversations));
}
