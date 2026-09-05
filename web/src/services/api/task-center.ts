import { normalizeBackendMediaJson, normalizeBackendMediaUrl, normalizeBackendMediaUrls } from "@/lib/app-base-path";
import { DREAMINA_SUBMIT_ERROR_MESSAGES, generationErrorMessage } from "@/lib/generation-error";
import { ApiError, apiClient, compactApiParams, request, type BackendEnvelope } from "@/services/api/request";
import {
    deleteLocalDreaminaGenerationTask,
    listLocalDreaminaGenerationTaskPage,
    queryLocalDreaminaGenerationTask,
    refreshLocalDreaminaGenerationTask,
    waitForLocalDreaminaGenerationTask,
    type LocalDreaminaGenerationTask,
} from "@/services/local-dreamina-generation";
import { isLocalDreaminaTaskId, projectLocalDreaminaDiagnosticLog, projectLocalDreaminaTask, stripLocalDreaminaTaskPrefix } from "@/services/local-dreamina-task-projection";

export type { BackendEnvelope } from "@/services/api/request";

export type TaskStatus = "queued" | "running" | "succeeded" | "failed" | "cancelled";
export type TaskBillingStatus = "reserved" | "running" | "settled" | "refunded" | "uncertain";
export type ProviderCancelStatus = "requested" | "confirmed" | "uncertain";
export type AgentSessionStatus = "active" | "completed" | "failed";
export type GenerationTaskResultState = "NOT_AVAILABLE" | "PENDING_MATERIALIZATION" | "MATERIALIZING" | "READY" | "FAILED_RETRYABLE" | "FAILED_PERMANENT";
export type GenerationTaskOutput = {
    outputIndex: number;
    mediaType: "image" | "video" | "audio";
    providerArtifactRef?: string;
    materializedAssetId?: string;
    materializationErrorCode?: string;
};

export type GenerationTask = {
    id: string;
    clientOperationId?: string;
    retryOf?: string;
    attemptGroupId?: string;
    sessionId?: string;
    projectId?: string;
    type: string;
    status: TaskStatus;
    progress?: number;
    stage?: string;
    prompt: string;
    operation?: string;
    provider?: string;
    model?: string;
    providerRequestId?: string;
    providerCancelStatus?: ProviderCancelStatus;
    providerCancelError?: string;
    providerCancelAttempts?: number;
    providerCancelRequestedAt?: string;
    providerCancelledAt?: string;
    errorCode?: string;
    officialStatus?: "pending" | "processing" | "completed" | "failed" | "cancelled";
    receiptRecorded?: boolean;
    previewUrl?: string;
    previewKind?: "image" | "video";
    inputJson?: string;
    resultJson?: string;
    resultState?: GenerationTaskResultState;
    outputs?: GenerationTaskOutput[];
    textDraft?: string;
    error?: string;
    attempts: number;
    startedAt?: string;
    completedAt?: string;
    createdAt: string;
    updatedAt: string;
    billing?: {
        amountMicrocredits: number;
        status: TaskBillingStatus;
    };
    clientContext?: {
        conversationId?: string;
        messageId?: string;
        nodeId?: string;
        batchIndex?: number;
        batchCount?: number;
    };
    created_at?: string;
    updated_at?: string;
};

export type ProviderTaskQueryResult = {
    task: GenerationTask;
    providerStatus: string;
    recovered: boolean;
    billingSettled: boolean;
};

export type TaskTextDelta = {
    id: string;
    taskId: string;
    sequence: number;
    content: string;
    byteCount: number;
    createdAt: string;
    expiresAt: string;
};

export type TaskTextReplay = {
    deltas: TaskTextDelta[];
    textDraft?: string;
    finalText?: string;
    complete: boolean;
};

export type AgentSession = {
    id: string;
    projectId?: string;
    status: AgentSessionStatus;
    prompt: string;
    canvasSnapshotJson?: string;
    canvasOpsJson?: string;
    createdAt: string;
    updatedAt: string;
};

export type AgentMessage = {
    id: string;
    sessionId: string;
    role: "user" | "assistant" | "system" | "tool" | string;
    content: string;
    payload?: string;
    createdAt: string;
};

export type TaskResult = {
    id: string;
    taskId: string;
    sessionId?: string;
    kind: string;
    url?: string;
    payload?: string;
    createdAt: string;
};

export type SessionFile = {
    id: string;
    sessionId: string;
    fileName: string;
    mimeType: string;
    size: number;
    createdAt: string;
};

export type TaskLog = {
    id: string;
    taskId: string;
    level: "info" | "warn" | "error";
    stage: string;
    errorCode?: string;
    provenance: "task_state" | "provider_observation" | "background_reconcile" | "manual_refresh" | "backend";
    observedAt?: string;
    createdAt: string;
};

export type AgentSessionDetail = {
    session: AgentSession;
    messages: AgentMessage[];
    tasks: GenerationTask[];
    results: TaskResult[];
};

export type CreateSessionInput = {
    projectId?: string;
    prompt: string;
    canvasSnapshot?: Record<string, unknown>;
    references?: string[];
    projectStyle?: { presetId: string; title: string; prompt: string };
    characters?: Array<{ assetId: string; versionId: string; name: string; definition: Record<string, unknown> }>;
    config?: Record<string, unknown>;
	logicalModelId?: string;
};

export type CreateTaskInput = {
    sessionId?: string;
    projectId?: string;
    type?: string;
    operation?: string;
    prompt: string;
    provider?: string;
    model?: string;
	logicalModelId?: string;
    input?: Record<string, unknown>;
};

const api = apiClient;

function normalizeGenerationTaskMedia(task: GenerationTask) {
    const previewUrl = task.previewUrl ? normalizeBackendMediaUrl(task.previewUrl) : task.previewUrl;
    const resultJson = task.resultJson ? normalizeBackendMediaJson(task.resultJson) : task.resultJson;
    const outputs = task.outputs ? normalizeBackendMediaUrls(task.outputs) : task.outputs;
    if (previewUrl === task.previewUrl && resultJson === task.resultJson && outputs === task.outputs) return task;
    return { ...task, previewUrl, resultJson, outputs };
}

function normalizeTaskResultMedia(result: TaskResult) {
    const url = result.url ? normalizeBackendMediaUrl(result.url) : result.url;
    const payload = result.payload ? normalizeBackendMediaJson(result.payload) : result.payload;
    return url === result.url && payload === result.payload ? result : { ...result, url, payload };
}

function normalizeAgentSessionDetailMedia(detail: AgentSessionDetail) {
    const tasks = detail.tasks.map(normalizeGenerationTaskMedia);
    const results = detail.results.map(normalizeTaskResultMedia);
    return tasks.every((task, index) => task === detail.tasks[index]) && results.every((result, index) => result === detail.results[index]) ? detail : { ...detail, tasks, results };
}

export function createAgentSession(input: CreateSessionInput) {
    return request<AgentSessionDetail>(api.post("/sessions", input)).then(normalizeAgentSessionDetailMedia).then((detail) => {
        detail.tasks.forEach((task) => notifyCanvasTaskCreated(task));
        return detail;
    });
}

export function queryAgentSession(id: string) {
    return request<AgentSessionDetail>(api.get(`/sessions/${encodeURIComponent(id)}`)).then(normalizeAgentSessionDetailMedia);
}

export function agentSessionFailureMessage(detail: AgentSessionDetail, fallback = "后端影视 Agent 会话失败") {
    for (let index = detail.tasks.length - 1; index >= 0; index -= 1) {
        const task = detail.tasks[index];
        if ((task.status === "failed" || task.status === "cancelled") && task.error?.trim()) return generationErrorMessage(task.error.trim());
    }
    for (let index = detail.messages.length - 1; index >= 0; index -= 1) {
        const message = detail.messages[index];
        if (message.role === "assistant" && message.content.trim()) return generationErrorMessage(message.content.trim());
    }
    return fallback;
}

export function downloadSessionResults(id: string) {
    return request<TaskResult[]>(api.get(`/sessions/${encodeURIComponent(id)}/results`)).then((results) => results.map(normalizeTaskResultMedia));
}

export function uploadAgentFile(sessionId: string, file: File) {
    const formData = new FormData();
    formData.append("sessionId", sessionId);
    formData.append("file", file);
    return request<SessionFile>(api.post("/files", formData));
}

export function createGenerationTask(input: CreateTaskInput) {
    return request<GenerationTask>(api.post("/tasks", input)).then(normalizeGenerationTaskMedia).then((task) => {
        notifyCanvasTaskCreated(task);
        // 创建任务时积分已被预占，不能等任务结束后才刷新可用余额。
        window.dispatchEvent(new CustomEvent("wallet:updated"));
        return task;
    });
}

export type GenerationTaskPageRequest = {
    limit: number;
    projectId?: string;
    activeOnly: boolean;
    cursor?: string;
};

export type CreateMessageGenerationTaskQuery = {
    conversationId: string;
    messageId: string;
    signal?: AbortSignal;
};

type GenerationTaskPage<T> = { tasks: T[]; nextCursor?: string };
type GenerationTaskListOptions = { projectId?: string; activeOnly?: boolean };
type GenerationTaskListDependencies = {
    listBackend?(limit: number, options?: GenerationTaskListOptions, signal?: AbortSignal): Promise<GenerationTask[]>;
    listLocal?(options?: GenerationTaskListOptions, signal?: AbortSignal): Promise<LocalDreaminaGenerationTask[]>;
    listBackendPage?(request: GenerationTaskPageRequest, signal?: AbortSignal): Promise<GenerationTaskPage<GenerationTask>>;
    listLocalPage?(request: GenerationTaskPageRequest, signal?: AbortSignal): Promise<GenerationTaskPage<LocalDreaminaGenerationTask>>;
};

const defaultGenerationTaskListDependencies: GenerationTaskListDependencies = {
    listBackendPage: async (page, signal) => ({
        tasks: await request<GenerationTask[]>(
            api.get("/tasks", {
                params: { limit: Math.min(page.limit, 100), projectId: page.projectId, activeOnly: page.activeOnly || undefined },
                signal,
            }),
        ),
    }),
    listLocalPage: (page, signal) => listLocalDreaminaGenerationTaskPage(page, {}, signal),
};

export function listGenerationTasksByCreateMessage(query: CreateMessageGenerationTaskQuery) {
    const conversationId = typeof query?.conversationId === "string" ? query.conversationId.trim() : "";
    const messageId = typeof query?.messageId === "string" ? query.messageId.trim() : "";
    if (!conversationId) throw new Error("创作消息任务查询缺少 conversationId");
    if (!messageId) throw new Error("创作消息任务查询缺少 messageId");

    // 后端以登录 Cookie 约束当前用户；此处不能混入浏览器本地任务缓存或其他消息的任务。
    return request<GenerationTask[]>(
        api.get("/tasks", {
            params: compactApiParams({ conversationId, messageId, limit: 100 }),
            signal: query.signal,
        }),
    ).then((tasks) => tasks.map(normalizeGenerationTaskMedia));
}

export async function listGenerationTasks(limit = 30, options?: { projectId?: string; activeOnly?: boolean }, dependencies: GenerationTaskListDependencies = defaultGenerationTaskListDependencies, signal?: AbortSignal) {
    if (signal?.aborted) throw new DOMException("Aborted", "AbortError");
    const boundedLimit = Number.isFinite(limit) ? Math.max(1, Math.min(10_000, Math.trunc(limit))) : 30;
    const baseRequest = {
        limit: Math.min(100, boundedLimit),
        ...(options?.projectId ? { projectId: options.projectId } : {}),
        activeOnly: options?.activeOnly === true,
    } satisfies GenerationTaskPageRequest;
    const backendPageReader =
        dependencies.listBackendPage ??
        (async (page: GenerationTaskPageRequest, pageSignal?: AbortSignal) => ({
            tasks:
                (await dependencies.listBackend?.(
                    page.limit,
                    {
                        ...(page.projectId ? { projectId: page.projectId } : {}),
                        activeOnly: page.activeOnly,
                    },
                    pageSignal,
                )) ?? [],
        }));
    const localPageReader =
        dependencies.listLocalPage ??
        (async (page: GenerationTaskPageRequest, pageSignal?: AbortSignal) => ({
            tasks:
                (await dependencies.listLocal?.(
                    {
                        ...(page.projectId ? { projectId: page.projectId } : {}),
                        activeOnly: page.activeOnly,
                    },
                    pageSignal,
                )) ?? [],
        }));
    const [backendTasks, localTasks] = await Promise.all([collectGenerationTaskPages(backendPageReader, baseRequest, boundedLimit, signal), collectGenerationTaskPages(localPageReader, baseRequest, boundedLimit, signal).catch(() => [])]);
    if (signal?.aborted) throw new DOMException("Aborted", "AbortError");
    return [...backendTasks.map(normalizeGenerationTaskMedia), ...localTasks.map((task) => projectLocalDreaminaTask(task))]
        .filter((task) => !options?.projectId || task.projectId === options.projectId)
        .filter((task) => !options?.activeOnly || task.status === "queued" || task.status === "running")
        .sort((left, right) => right.updatedAt.localeCompare(left.updatedAt) || right.id.localeCompare(left.id))
        .slice(0, boundedLimit);
}

async function collectGenerationTaskPages<T>(readPage: (request: GenerationTaskPageRequest, signal?: AbortSignal) => Promise<GenerationTaskPage<T>>, baseRequest: GenerationTaskPageRequest, limit: number, signal?: AbortSignal) {
    const items: T[] = [];
    const seenCursors = new Set<string>();
    let cursor: string | undefined;
    do {
        if (signal?.aborted) throw new DOMException("Aborted", "AbortError");
        const response = await readPage({ ...baseRequest, ...(cursor ? { cursor } : {}) }, signal);
        const page = Array.isArray(response) ? { tasks: response as T[] } : response;
        items.push(...page.tasks);
        if (!page.nextCursor || seenCursors.has(page.nextCursor)) break;
        seenCursors.add(page.nextCursor);
        cursor = page.nextCursor;
    } while (items.length < limit);
    return items.slice(0, limit);
}

export function queryGenerationTask(id: string, options?: { signal?: AbortSignal }) {
    if (isLocalDreaminaTaskId(id)) {
        return queryLocalDreaminaGenerationTask(stripLocalDreaminaTaskPrefix(id), undefined, {}, options?.signal).then((task) => projectLocalDreaminaTask(task));
    }
    return request<GenerationTask>(api.get(`/tasks/${encodeURIComponent(id)}`, { signal: options?.signal })).then(normalizeGenerationTaskMedia);
}

export function waitForLocalGenerationTask(id: string, options?: { signal?: AbortSignal }) {
    if (!isLocalDreaminaTaskId(id)) return Promise.reject(new Error("当前任务不是本机即梦任务"));
    return waitForLocalDreaminaGenerationTask(stripLocalDreaminaTaskPrefix(id), undefined, {}, options?.signal).then((task) => projectLocalDreaminaTask(task));
}

export function splitGenerationTaskObservationIds(ids: readonly string[]) {
    return {
        localWaitIds: ids.filter(isLocalDreaminaTaskId),
        remotePollIds: ids.filter((id) => !isLocalDreaminaTaskId(id)),
    };
}

type GenerationTaskSubscriptionDependencies = {
    queryTask(id: string, options?: { signal?: AbortSignal }): Promise<GenerationTask>;
    waitTask(id: string, options?: { signal?: AbortSignal; initialTask?: GenerationTask; onTaskUpdate?: (task: GenerationTask) => void }): Promise<GenerationTask>;
    maxRetries?: number;
    retryDelayMs?: (retryCount: number) => number;
};

export type GenerationTaskSubscriptionError = {
    taskId: string;
    error: unknown;
    retryable: boolean;
    retryCount: number;
    willRetry: boolean;
};

export type GenerationTaskSubscriptionOptions = {
    onError?: (event: GenerationTaskSubscriptionError) => void;
};

const MAX_TASK_OBSERVATION_RETRIES = 3;

function isRetryableTaskObservationError(error: unknown) {
    if (isAbortError(error)) return false;
    const status = errorStatus(error);
    // 未认证、无权限和任务已删除等 4xx 结果不会自行恢复，不能继续轮询。
    if (status !== undefined && status >= 400 && status < 500 && status !== 408 && status !== 425 && status !== 429) return false;
    if (error instanceof ApiError) return error.retryable || isTemporaryNetworkError(error.cause) || (status === undefined && isTemporaryNetworkError(error));
    return isTemporaryNetworkError(error);
}

function errorStatus(error: unknown) {
    if (!error || typeof error !== "object") return undefined;
    const status = (error as { status?: unknown }).status;
    return typeof status === "number" && Number.isFinite(status) ? status : undefined;
}

function isAbortError(error: unknown) {
    return error instanceof DOMException && error.name === "AbortError";
}

function isTemporaryNetworkError(error: unknown): boolean {
    if (!error || typeof error !== "object") return false;
    const candidate = error as { name?: unknown; message?: unknown; code?: unknown };
    if (candidate.name === "AbortError") return false;
    const code = typeof candidate.code === "string" ? candidate.code.toUpperCase() : "";
    if (["ECONNABORTED", "ECONNRESET", "ECONNREFUSED", "ENETUNREACH", "ETIMEDOUT", "EAI_AGAIN", "ERR_NETWORK", "ERR_CONNECTION_RESET"].includes(code)) return true;
    const message = typeof candidate.message === "string" ? candidate.message : "";
    return /(?:network(?:\s+error)?|failed to fetch|load failed|timeout|timed out|temporar(?:y|ily)|connection (?:reset|refused|lost)|offline)/i.test(message);
}

export function createGenerationTaskSubscriptionService(dependencies: GenerationTaskSubscriptionDependencies) {
    const maxRetries = Number.isSafeInteger(dependencies.maxRetries) ? Math.max(0, dependencies.maxRetries!) : MAX_TASK_OBSERVATION_RETRIES;
    const retryDelayMs = dependencies.retryDelayMs ?? ((retryCount: number) => Math.min(1_000 * 2 ** retryCount, 30_000));
    type Entry = {
        listeners: Map<(task: GenerationTask) => void, GenerationTaskSubscriptionOptions["onError"]>;
        latest?: GenerationTask;
        observation?: Promise<void>;
        controller?: AbortController;
        retryTimer?: ReturnType<typeof setTimeout>;
        retryCount: number;
        observationFailed?: GenerationTaskSubscriptionError;
    };
    const entries = new Map<string, Entry>();
    const isTerminal = (task?: GenerationTask) => task?.status === "succeeded" || task?.status === "failed" || task?.status === "cancelled";
    const notifyTaskListener = (listener: (task: GenerationTask) => void, task: GenerationTask) => {
        try {
            listener(task);
        } catch (error) {
            // 单个页面订阅者的渲染/状态异常不能让同一任务的其他订阅者停止观察。
            console.error("生成任务订阅回调失败", error);
        }
    };
    const notifyErrorListener = (onError: GenerationTaskSubscriptionOptions["onError"], event: GenerationTaskSubscriptionError) => {
        try {
            onError?.(event);
        } catch (error) {
            console.error("生成任务订阅错误回调失败", error);
        }
    };
    const publish = (entry: Entry, task: GenerationTask) => {
        entry.latest = task;
        for (const listener of entry.listeners.keys()) notifyTaskListener(listener, task);
    };
    const publishError = (entry: Entry, event: GenerationTaskSubscriptionError) => {
        for (const onError of entry.listeners.values()) notifyErrorListener(onError, event);
    };
    const disposeIdleEntry = (id: string, entry: Entry) => {
        if (entries.get(id) === entry && entry.listeners.size === 0 && !entry.observation && !entry.retryTimer) entries.delete(id);
    };
    let observe: (id: string, entry: Entry) => void;
    const scheduleRetry = (id: string, entry: Entry) => {
        if (entry.listeners.size === 0 || entry.retryTimer || isTerminal(entry.latest) || entry.observationFailed || entry.retryCount >= maxRetries) return false;
        const requestedDelay = retryDelayMs(entry.retryCount);
        const delay = Number.isFinite(requestedDelay) ? Math.max(0, requestedDelay) : 0;
        entry.retryCount += 1;
        entry.retryTimer = setTimeout(() => {
            entry.retryTimer = undefined;
            observe(id, entry);
        }, delay);
        return true;
    };
    observe = (id: string, entry: Entry) => {
        if (entry.observation || entry.retryTimer || entry.listeners.size === 0 || isTerminal(entry.latest) || entry.observationFailed) return;
        const controller = new AbortController();
        entry.controller = controller;
        const observation = (async () => {
            const initial = await dependencies.queryTask(id, { signal: controller.signal });
            // 重试预算属于整个订阅生命周期；等待阶段失败时不能因这次查询成功而反复清零。
            publish(entry, initial);
            if (isTerminal(initial)) return;
            const terminal = await dependencies.waitTask(id, {
                signal: controller.signal,
                initialTask: initial,
                onTaskUpdate: (task) => publish(entry, task),
            });
            publish(entry, terminal);
        })();
        entry.observation = observation;
        void observation.catch((error) => {
            // waitForGenerationTask 会先发布失败/取消终态，再以异常结束等待；
            // 该终态已由 listener 消费，不能再误报为“状态同步失败”。
            if (controller.signal.aborted || isTerminal(entry.latest)) return;
            const retryable = isRetryableTaskObservationError(error);
            const willRetry = retryable && scheduleRetry(id, entry);
            const event: GenerationTaskSubscriptionError = { taskId: id, error, retryable, retryCount: entry.retryCount, willRetry };
            if (!willRetry) entry.observationFailed = event;
            publishError(entry, event);
        }).finally(() => {
            if (entry.observation === observation) {
                entry.observation = undefined;
                entry.controller = undefined;
            }
            if (entry.listeners.size > 0 && !entry.retryTimer && !isTerminal(entry.latest) && !entry.observationFailed) observe(id, entry);
            disposeIdleEntry(id, entry);
        });
    };
    return {
        subscribe(ids: readonly string[], listener: (task: GenerationTask) => void, options: GenerationTaskSubscriptionOptions = {}) {
            const uniqueIds = [...new Set(ids)];
            for (const id of uniqueIds) {
                const entry = entries.get(id) ?? { listeners: new Map<(task: GenerationTask) => void, GenerationTaskSubscriptionOptions["onError"]>(), retryCount: 0 };
                entries.set(id, entry);
                entry.listeners.set(listener, options.onError);
                if (entry.latest) notifyTaskListener(listener, entry.latest);
                if (entry.observationFailed) notifyErrorListener(options.onError, entry.observationFailed);
                observe(id, entry);
            }
            return () => {
                for (const id of uniqueIds) {
                    const entry = entries.get(id);
                    if (!entry) continue;
                    entry.listeners.delete(listener);
                    if (entry.listeners.size === 0) {
                        entry.controller?.abort();
                        if (entry.retryTimer) {
                            clearTimeout(entry.retryTimer);
                            entry.retryTimer = undefined;
                        }
                    }
                    disposeIdleEntry(id, entry);
                }
            };
        },
    };
}

const generationTaskSubscriptionService = createGenerationTaskSubscriptionService({
    queryTask: (id, options) => queryGenerationTask(id, options),
    waitTask: (id, options) => waitForGenerationTask(id, options),
});

export function subscribeGenerationTasks(ids: readonly string[], listener: (task: GenerationTask) => void, options?: GenerationTaskSubscriptionOptions) {
    return generationTaskSubscriptionService.subscribe(ids, listener, options);
}

export function appendTaskTextDelta(id: string, content: string) {
    return request<TaskTextDelta>(api.post(`/tasks/${encodeURIComponent(id)}/text-deltas`, { content }));
}

export function completeTextReplayTask(id: string, text: string) {
    return request<GenerationTask>(api.post(`/tasks/${encodeURIComponent(id)}/text-replay-complete`, { text })).then(normalizeGenerationTaskMedia);
}

export function queryTaskTextReplay(id: string, after = 0) {
    return request<TaskTextReplay>(api.get(`/tasks/${encodeURIComponent(id)}/text-deltas`, { params: { after } }));
}

export function retryGenerationTask(id: string) {
    return request<GenerationTask>(api.post(`/tasks/${encodeURIComponent(id)}/retry`)).then(normalizeGenerationTaskMedia);
}

export function queryFailedVideoProviderTask(id: string) {
    return request<ProviderTaskQueryResult>(api.post(`/tasks/${encodeURIComponent(id)}/query-provider`)).then((result) => ({ ...result, task: normalizeGenerationTaskMedia(result.task) }));
}

export function refreshGenerationTaskStatus(id: string, options?: { signal?: AbortSignal }) {
    if (!isLocalDreaminaTaskId(id)) return Promise.reject(new Error("当前任务不支持手动更新官方状态"));
    return refreshLocalDreaminaGenerationTask(stripLocalDreaminaTaskPrefix(id), {}, options?.signal).then((task) => projectLocalDreaminaTask(task));
}

export function deleteGenerationTask(id: string) {
    if (!isLocalDreaminaTaskId(id)) return Promise.reject(new Error("当前任务不支持删除"));
    return deleteLocalDreaminaGenerationTask(stripLocalDreaminaTaskPrefix(id));
}

export async function listTaskLogs(id: string) {
    if (isLocalDreaminaTaskId(id)) {
        const task = await queryGenerationTask(id);
        return [projectGenerationTaskSafeLog(task)];
    }
    const raw = await request<Array<{ level?: unknown; message?: unknown; payload?: unknown; createdAt?: unknown }>>(api.get(`/tasks/${encodeURIComponent(id)}/logs`));
    return raw.map((log, index) => projectBackendSafeTaskLog(id, log, index));
}

export function formatTaskLog(log: TaskLog) {
    return [`stage=${log.stage}`, ...(log.errorCode ? [`error=${log.errorCode}`] : []), `provenance=${log.provenance}`, ...(log.observedAt ? [`observedAt=${log.observedAt}`] : [])].join(" ");
}

function projectGenerationTaskSafeLog(task: GenerationTask): TaskLog {
    const diagnostic = projectLocalDreaminaDiagnosticLog({
        level: task.errorCode || task.status === "failed" || task.status === "cancelled" ? "error" : "info",
        stage: task.stage || task.status,
        errorCode: task.errorCode,
        provenance: task.officialStatus ? "provider_observation" : "task_state",
        observedAt: task.updatedAt,
    });
    return {
        id: `safe:${task.id}:${task.updatedAt}`,
        taskId: task.id,
        ...diagnostic,
        createdAt: diagnostic.observedAt,
    };
}

export function projectBackendSafeTaskLog(taskId: string, raw: { level?: unknown; message?: unknown; payload?: unknown; createdAt?: unknown }, index: number): TaskLog {
    const text = [raw.message, raw.payload].filter((value): value is string => typeof value === "string").join(" ");
    const stage = safeTaskLogStage(text) || "backend_event";
    const errorCode = safeTaskLogErrorCode(text);
    const createdAt = typeof raw.createdAt === "string" && Number.isFinite(Date.parse(raw.createdAt)) ? raw.createdAt : "1970-01-01T00:00:00.000Z";
    return {
        id: `safe:${taskId}:${index}`,
        taskId,
        level: raw.level === "error" ? "error" : raw.level === "warn" ? "warn" : "info",
        stage,
        ...(errorCode ? { errorCode } : {}),
        provenance: "backend",
        createdAt,
    };
}

function safeTaskLogStage(value: unknown) {
    if (typeof value !== "string") return undefined;
    const match = value.match(/(?:^|[^a-z0-9_])(queued|submitting|submitted|generating|submission_unknown|processing|completed|succeeded|failed|cancelled)(?:$|[^a-z0-9_])/i);
    return match?.[1]?.toLowerCase();
}

const SAFE_TASK_LOG_ERROR_CODES = new Set([...Object.keys(DREAMINA_SUBMIT_ERROR_MESSAGES), "dreamina_query_failed", "dreamina_submission_unknown", "dreamina_reference_invalid"]);

function safeTaskLogErrorCode(value: unknown) {
    if (typeof value !== "string") return undefined;
    for (const match of value.matchAll(/(?:dreamina|local_generation|model|provider)_[a-z0-9_]{2,80}/gi)) {
        const errorCode = match[0].toLowerCase();
        if (SAFE_TASK_LOG_ERROR_CODES.has(errorCode)) return errorCode;
    }
    return undefined;
}

export async function waitForGenerationTask(id: string, options?: { signal?: AbortSignal; intervalMs?: number; timeoutMs?: number; initialTask?: GenerationTask; onTaskUpdate?: (task: GenerationTask) => void }) {
    if (isLocalDreaminaTaskId(id)) {
        try {
            const task = await waitForLocalGenerationTask(id, { signal: options?.signal });
            options?.onTaskUpdate?.(task);
            if (task.status === "succeeded") return task;
            if (task.status === "failed" || task.status === "cancelled") {
                throw new Error(task.error ? generationErrorMessage(task.error) : `任务${task.status === "cancelled" ? "已取消" : "失败"}`);
            }
            throw new Error("本机即梦任务等待提前结束");
        } catch (error) {
            if (options?.signal?.aborted) throw new DOMException("Aborted", "AbortError");
            throw error;
        }
    }
    const startedAt = Date.now();
    const intervalMs = options?.intervalMs || 2000;
    let lastTask = options?.initialTask;
    let lastQueryError: unknown;
    let consecutiveRetryableQueryFailures = 0;
    try {
        while (Date.now() - startedAt < (options?.timeoutMs || taskWaitTimeoutMs(lastTask))) {
            if (options?.signal?.aborted) throw new DOMException("Aborted", "AbortError");
            let task: GenerationTask;
            try {
                task = await queryGenerationTask(id, { signal: options?.signal });
                lastTask = task;
                lastQueryError = undefined;
                // 只有拿到可用任务状态才恢复内部容错额度，避免断网时持续轮询到任务超时。
                consecutiveRetryableQueryFailures = 0;
                options?.onTaskUpdate?.(task);
            } catch (error) {
                if (!isRetryableTaskObservationError(error)) throw error;
                lastQueryError = error;
                consecutiveRetryableQueryFailures += 1;
                if (consecutiveRetryableQueryFailures > MAX_TASK_OBSERVATION_RETRIES) throw error;
                await delay(intervalMs, options?.signal);
                continue;
            }
            if (task.status === "succeeded") {
                window.dispatchEvent(new CustomEvent("wallet:updated"));
                return task;
            }
            if (task.status === "failed" || task.status === "cancelled") {
                window.dispatchEvent(new CustomEvent("wallet:updated"));
                throw new Error(task.error ? generationErrorMessage(task.error) : `任务${task.status === "cancelled" ? "已取消" : "失败"}`);
            }
            await delay(intervalMs, options?.signal);
        }
    } catch (error) {
        if (options?.signal?.aborted) {
            // Abort 只停止当前页面的状态监听，不能把已发起的上游任务改成取消状态。
            throw new DOMException("Aborted", "AbortError");
        }
        throw error;
    }
    throw new Error(lastQueryError instanceof Error ? `任务状态同步失败：${lastQueryError.message}` : "任务执行超时，请稍后重试");
}

function taskWaitTimeoutMs(task?: GenerationTask) {
    const type = task?.type || "";
    if (type.includes("storyboard")) return 13 * 60 * 1000;
    if (type.includes("video")) return 32 * 60 * 1000;
    if (type.includes("image")) return 10 * 60 * 1000;
    if (type.includes("text") || type.includes("audio")) return 12 * 60 * 1000;
    return 10 * 60 * 1000;
}

function delay(ms: number, signal?: AbortSignal) {
    return new Promise<void>((resolve, reject) => {
        const timer = window.setTimeout(resolve, ms);
        signal?.addEventListener(
            "abort",
            () => {
                window.clearTimeout(timer);
                reject(new DOMException("Aborted", "AbortError"));
            },
            { once: true },
        );
    });
}

function notifyCanvasTaskCreated(task: GenerationTask) {
    if (typeof window === "undefined" || !task.projectId) return;
    window.dispatchEvent(new CustomEvent("canvas:task-created", { detail: { task } }));
}
