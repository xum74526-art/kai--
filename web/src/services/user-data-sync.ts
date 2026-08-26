import { getMediaBlob } from "@/services/file-storage";
import { getImageBlob } from "@/services/image-storage";
import { deleteRemoteAsset, deleteRemoteCanvasProject, getRemoteUserDataSnapshot, upsertRemoteAsset, upsertRemoteCanvasProject } from "@/services/api/user-data";
import { resourceFileUrl, resourceIdFromStorageKey, resourceStorageKey, uploadResourceFile } from "@/services/api/resources";
import type { Asset } from "@/stores/use-asset-store";
import { useAssetStore } from "@/stores/use-asset-store";
import type { CanvasProject } from "@/stores/canvas/use-canvas-store";
import { useCanvasStore } from "@/stores/canvas/use-canvas-store";
import { normalizeBackendMediaUrls } from "@/lib/app-base-path";

let activeRemoteUserId = "";
let applyingRemoteState = false;
let syncTimer: number | null = null;
let syncPromise: Promise<void> | null = null;
let syncQueued = false;
let syncPauseCount = 0;
let syncPauseTail: Promise<void> = Promise.resolve();
let resumeSync: (() => void) | null = null;
let syncResumed: Promise<void> | null = null;
const activeRemoteSyncOperations = new Set<Promise<void>>();
let subscriptionsInstalled = false;
let remoteAssetVersions = new Map<string, string>();
let remoteProjectVersions = new Map<string, string>();
let knownRemoteAssetVersions = new Map<string, string>();
let knownRemoteProjectVersions = new Map<string, string>();
let knownRemoteAssetIds = new Set<string>();
let knownRemoteProjectIds = new Set<string>();
let pendingDeletedAssetIds = new Set<string>();
let pendingDeletedProjectIds = new Set<string>();
let syncRetryAttempt = 0;
let remoteSnapshotFreshForPush = false;

const LOCAL_STORAGE_KEY_PATTERN = /^(image|generation-image|video|generation-video|audio|generation-audio|file|video-reference|audio-reference):/;
const REMOTE_KNOWN_IDS_STORAGE_PREFIX = "kai:remote-user-data-known-v1:";

export async function syncRemoteUserData(userId?: string | null) {
    await runRemoteUserDataSyncOperation(async () => {
        activeRemoteUserId = userId || "";
        if (!activeRemoteUserId) return;
        applyingRemoteState = true;
        try {
            // 登录只拉一次聚合快照。摘要列表再逐条请求详情会把 N 条数据放大成 2N+2 个请求，
            // 并且会在登录阶段同时触发大量媒体解析，任何一项失败都会污染登录结果。
            const snapshot = normalizeBackendMediaUrls(await getRemoteUserDataSnapshot());
            const known = readKnownRemoteIds(activeRemoteUserId);
            knownRemoteProjectIds = known.projects;
            knownRemoteAssetIds = known.assets;
            knownRemoteProjectVersions = known.projectVersions;
            knownRemoteAssetVersions = known.assetVersions;
            pendingDeletedProjectIds = known.deletedProjects;
            pendingDeletedAssetIds = known.deletedAssets;
            const localProjects = useCanvasStore.getState().projects;
            const localAssets = useAssetStore.getState().assets;
            const mergedProjects = mergeRemoteSnapshot(localProjects, snapshot.projects, knownRemoteProjectIds, pendingDeletedProjectIds, knownRemoteProjectVersions);
            // 这里只合并结构化素材数据，不在登录阶段解析图片/视频/音频 URL；媒体由实际使用方按需解析。
            const mergedAssets = mergeRemoteSnapshot(localAssets, snapshot.assets, knownRemoteAssetIds, pendingDeletedAssetIds, knownRemoteAssetVersions);
            useCanvasStore.getState().replaceProjects(mergedProjects);
            useAssetStore.getState().replaceAssets(mergedAssets);
            remoteProjectVersions = versionMap(snapshot.projects);
            remoteAssetVersions = versionMap(snapshot.assets);
            rememberRemoteIds(snapshot.projects, snapshot.assets);
            remoteSnapshotFreshForPush = true;
        } finally {
            applyingRemoteState = false;
        }
    });
    // 首次登录可能带有尚未创建到云端的本地画布；先完成一次 upsert，避免详情页保存/分享先于项目创建。
    try {
        await saveRemoteUserDataNow();
    } catch (error) {
        console.warn("登录后画布首次同步失败，保留本地项目等待重试", error);
    }
    scheduleRemoteUserDataSync();
}

export function installRemoteUserDataAutoSync() {
    if (subscriptionsInstalled) return;
    subscriptionsInstalled = true;
    useCanvasStore.subscribe((state, previous) => {
        if (!applyingRemoteState) rememberLocalDeletions(previous.projects, state.projects, knownRemoteProjectIds, pendingDeletedProjectIds);
        if (state.projects !== previous.projects) scheduleRemoteUserDataSync();
    });
    useAssetStore.subscribe((state, previous) => {
        if (!applyingRemoteState) rememberLocalDeletions(previous.assets, state.assets, knownRemoteAssetIds, pendingDeletedAssetIds);
        if (state.assets !== previous.assets) scheduleRemoteUserDataSync();
    });
}

export function resetRemoteUserDataSync() {
    activeRemoteUserId = "";
    remoteAssetVersions.clear();
    remoteProjectVersions.clear();
    knownRemoteAssetVersions.clear();
    knownRemoteProjectVersions.clear();
    knownRemoteAssetIds.clear();
    knownRemoteProjectIds.clear();
    pendingDeletedAssetIds.clear();
    pendingDeletedProjectIds.clear();
    syncRetryAttempt = 0;
    remoteSnapshotFreshForPush = false;
    if (syncTimer) {
        window.clearTimeout(syncTimer);
        syncTimer = null;
    }
    syncQueued = false;
}

function waitForRemoteUserDataSyncResume() {
    if (!syncPauseCount) return Promise.resolve();
    if (!syncResumed) {
        syncResumed = new Promise<void>((resolve) => {
            resumeSync = resolve;
        });
    }
    return syncResumed;
}

async function runRemoteUserDataSyncOperation<T>(operation: () => Promise<T>): Promise<T> {
    while (syncPauseCount) await waitForRemoteUserDataSyncResume();
    let finish!: () => void;
    const active = new Promise<void>((resolve) => {
        finish = resolve;
    });
    activeRemoteSyncOperations.add(active);
    try {
        return await operation();
    } finally {
        activeRemoteSyncOperations.delete(active);
        finish();
    }
}

export async function withRemoteUserDataSyncPaused<T>(operation: () => Promise<T>): Promise<T> {
    syncPauseCount += 1;
    if (syncTimer) {
        window.clearTimeout(syncTimer);
        syncTimer = null;
        syncQueued = true;
    }
    let releaseTurn!: () => void;
    const previousTurn = syncPauseTail;
    syncPauseTail = new Promise<void>((resolve) => {
        releaseTurn = resolve;
    });
    await previousTurn;
    try {
        if (activeRemoteSyncOperations.size) {
            await Promise.allSettled([...activeRemoteSyncOperations]);
        }
        if (syncPromise) await syncPromise;
        return await operation();
    } finally {
        releaseTurn();
        syncPauseCount -= 1;
        if (!syncPauseCount) {
            const resolve = resumeSync;
            resumeSync = null;
            syncResumed = null;
            resolve?.();
            if (syncQueued) scheduleRemoteUserDataSync();
        }
    }
}

export function scheduleRemoteUserDataSync() {
    scheduleRemoteUserDataSyncAfter(1200);
}

function scheduleRemoteUserDataSyncAfter(delayMs: number) {
    if (!activeRemoteUserId || applyingRemoteState) return;
    if (syncPauseCount) {
        syncQueued = true;
        if (syncTimer) {
            window.clearTimeout(syncTimer);
            syncTimer = null;
        }
        return;
    }
    if (syncPromise) {
        syncQueued = true;
        return;
    }
    if (syncTimer) window.clearTimeout(syncTimer);
    syncTimer = window.setTimeout(() => {
        syncTimer = null;
        void saveRemoteUserDataNow().catch((error) => console.warn("云端自动同步失败", error));
    }, delayMs);
}

export async function createCanvasProjectWithRemoteSync(title: string, projectId?: string, initialContent?: Partial<Pick<CanvasProject, "nodes" | "connections">>) {
    const id = useCanvasStore.getState().createProject(title, projectId);
    if (initialContent) useCanvasStore.getState().updateProject(id, initialContent);
    if (!activeRemoteUserId) return { id, syncError: new Error("尚未建立云端同步会话") };
    try {
        await saveRemoteUserDataNow();
        return { id };
    } catch (syncError) {
        scheduleRemoteUserDataSync();
        return { id, syncError };
    }
}

export async function deleteAssetWithRemoteSync(id: string) {
    await runRemoteUserDataSyncOperation(async () => {
        if (activeRemoteUserId) {
            await deleteRemoteAsset(id);
            remoteAssetVersions.delete(id);
            pendingDeletedAssetIds.delete(id);
        }
        applyingRemoteState = true;
        try {
            useAssetStore.getState().removeAsset(id);
        } finally {
            applyingRemoteState = false;
        }
        persistKnownRemoteIds();
    });
}

export async function saveRemoteUserDataNow() {
    if (!activeRemoteUserId) return;
    if (syncPauseCount) {
        syncQueued = true;
        await waitForRemoteUserDataSyncResume();
        return saveRemoteUserDataNow();
    }
    if (syncPromise) {
        syncQueued = true;
        return syncPromise;
    }
    syncPromise = drainRemoteUserDataChanges();
    let failed = false;
    try {
        await syncPromise;
        syncRetryAttempt = 0;
    } catch (error) {
        failed = true;
        throw error;
    } finally {
        syncPromise = null;
        if (failed && activeRemoteUserId) {
            syncRetryAttempt += 1;
            scheduleRemoteUserDataSyncAfter(Math.min(30_000, 2_000 * 2 ** Math.min(syncRetryAttempt - 1, 4)));
        }
    }
}

async function drainRemoteUserDataChanges() {
    do {
        syncQueued = false;
        await saveRemoteUserDataBatch();
    } while (syncQueued);
}

async function saveRemoteUserDataBatch() {
    try {
        // 仅在完整聚合快照成功返回后应用删除推断；这不是服务端 tombstone 的原子替代方案。
        // 登录刚拉取的快照可复用一次，后续推送前重新确认远端现状，让旧设备识别另一设备的删除。
        if (remoteSnapshotFreshForPush) remoteSnapshotFreshForPush = false;
        else await refreshRemoteStateBeforePush();
        const currentProjects = useCanvasStore.getState().projects;
        const currentAssets = useAssetStore.getState().assets;
        const dirtyProjects = currentProjects.filter((item) => !sameVersion(remoteProjectVersions.get(item.id), item.updatedAt));
        const dirtyAssets = currentAssets.filter((item) => !sameVersion(remoteAssetVersions.get(item.id), item.updatedAt));
        const deletedProjectIds = missingIds(remoteProjectVersions, currentProjects);
        const deletedAssetIds = missingIds(remoteAssetVersions, currentAssets);
        if (!dirtyProjects.length && !dirtyAssets.length && !deletedProjectIds.length && !deletedAssetIds.length) return;
        const uploaded = new Map<string, string>();
        const projects = await prepareRemoteCanvasProjects(dirtyProjects, uploaded);
        const assets = await prepareRemoteAssets(dirtyAssets, uploaded);
        applyingRemoteState = true;
        if (projects.length) useCanvasStore.getState().replaceProjects(mergePreparedItems(useCanvasStore.getState().projects, dirtyProjects, projects, uploaded));
        if (assets.length) useAssetStore.getState().replaceAssets(mergePreparedItems(useAssetStore.getState().assets, dirtyAssets, assets, uploaded));
        applyingRemoteState = false;
        // SQLite 和接口频控都要求写入保持有界；逐项提交还能准确记录已完成版本。
        const latestProjectIds = new Set(useCanvasStore.getState().projects.map((item) => item.id));
        const latestAssetIds = new Set(useAssetStore.getState().assets.map((item) => item.id));
        for (const project of projects.filter((item) => latestProjectIds.has(item.id))) {
            await upsertRemoteCanvasProject(project);
            remoteProjectVersions.set(project.id, project.updatedAt);
            knownRemoteProjectIds.add(project.id);
            knownRemoteProjectVersions.set(project.id, project.updatedAt);
            // 删除可能发生在首次 upsert 等待期间；此时订阅器尚不知道它已存在于远端。
            // 在同一批次补记 tombstone 并立即清理，避免下一次快照把它重新带回本地。
            if (!useCanvasStore.getState().projects.some((item) => item.id === project.id)) {
                pendingDeletedProjectIds.add(project.id);
                persistKnownRemoteIds();
                await deleteRemoteCanvasProject(project.id);
                remoteProjectVersions.delete(project.id);
                pendingDeletedProjectIds.delete(project.id);
            }
        }
        for (const asset of assets.filter((item) => latestAssetIds.has(item.id))) {
            await upsertRemoteAsset(asset);
            remoteAssetVersions.set(asset.id, asset.updatedAt);
            knownRemoteAssetIds.add(asset.id);
            knownRemoteAssetVersions.set(asset.id, asset.updatedAt);
            if (!useAssetStore.getState().assets.some((item) => item.id === asset.id)) {
                pendingDeletedAssetIds.add(asset.id);
                persistKnownRemoteIds();
                await deleteRemoteAsset(asset.id);
                remoteAssetVersions.delete(asset.id);
                pendingDeletedAssetIds.delete(asset.id);
            }
        }
        for (const id of deletedProjectIds) {
            await deleteRemoteCanvasProject(id);
            remoteProjectVersions.delete(id);
            pendingDeletedProjectIds.delete(id);
        }
        for (const id of deletedAssetIds) {
            await deleteRemoteAsset(id);
            remoteAssetVersions.delete(id);
            pendingDeletedAssetIds.delete(id);
        }
        persistKnownRemoteIds();
    } finally {
        applyingRemoteState = false;
    }
}

async function prepareRemoteAssets(assets: Asset[], uploaded: Map<string, string>) {
    const result: Asset[] = [];
    for (const asset of assets) result.push(await ensureRemoteResourceReferences(asset, uploaded));
    return result;
}

async function prepareRemoteCanvasProjects(projects: CanvasProject[], uploaded: Map<string, string>) {
    const result: CanvasProject[] = [];
    for (const project of projects) result.push(await ensureRemoteResourceReferences(project, uploaded));
    return result;
}

async function ensureRemoteResourceReferences<T>(value: T, uploaded = new Map<string, string>()): Promise<T> {
    if (!value || typeof value !== "object") return value;
    if (Array.isArray(value)) {
        const result: unknown[] = [];
        for (const item of value) result.push(await ensureRemoteResourceReferences(item, uploaded));
        return result as T;
    }

    const next: Record<string, unknown> = {};
    for (const [key, child] of Object.entries(value)) {
        next[key] = await ensureRemoteResourceReferences(child, uploaded);
    }

    const storageKey = typeof next.storageKey === "string" ? next.storageKey : "";
    const remoteResourceId = resourceIdFromStorageKey(storageKey);
    if (remoteResourceId) return applyResourceReference(next, storageKey) as T;

    if (!isLocalStorageKey(storageKey)) {
        const inline = inlineMediaDataUrl(next);
        if (!inline) return next as T;
        const resourceStorage = await uploadInlineDataUrl(inline);
        return applyResourceReference(next, resourceStorage) as T;
    }

    const cached = uploaded.get(storageKey);
    const resourceStorage = cached || (await uploadLocalStorageKey(storageKey, next));
    uploaded.set(storageKey, resourceStorage);
    return applyResourceReference(next, resourceStorage) as T;
}

function applyResourceReference(payload: Record<string, unknown>, storageKey: string) {
    const url = resourceFileUrl(storageKey.slice("resource:".length));
    payload.storageKey = storageKey;
    for (const key of ["content", "dataUrl", "url", "coverUrl"]) {
        if (typeof payload[key] === "string") payload[key] = url;
    }
    return payload;
}

function inlineMediaDataUrl(payload: Record<string, unknown>) {
    for (const key of ["dataUrl", "content", "url", "coverUrl"]) {
        const value = payload[key];
        if (typeof value === "string" && /^data:(image|video|audio)\//i.test(value)) return value;
    }
    return "";
}

async function uploadInlineDataUrl(dataUrl: string) {
    const response = await fetch(dataUrl);
    if (!response.ok) throw new Error(`内联媒体读取失败（HTTP ${response.status}）`);
    const blob = await response.blob();
    const kind: "image" | "video" | "audio" | "file" = blob.type.startsWith("image/") ? "image" : blob.type.startsWith("video/") ? "video" : blob.type.startsWith("audio/") ? "audio" : "file";
    const resource = await uploadResourceFile(blob, kind);
    return resourceStorageKey(resource.id);
}

async function uploadLocalStorageKey(storageKey: string, payload: Record<string, unknown>) {
    const blob = isImageStorageKey(storageKey) ? await getImageBlob(storageKey) : await getMediaBlob(storageKey);
    if (!blob) throw new Error(`本地媒体文件已丢失，无法同步：${storageKey}`);
    const kind = blob.type.startsWith("image/") ? "image" : blob.type.startsWith("video/") ? "video" : blob.type.startsWith("audio/") ? "audio" : "file";
    const resource = await uploadResourceFile(blob, kind, {
        width: numberValue(payload.naturalWidth) || numberValue(payload.width),
        height: numberValue(payload.naturalHeight) || numberValue(payload.height),
        durationMs: numberValue(payload.durationMs),
    });
    return resourceStorageKey(resource.id);
}

function mergeById<T extends { id?: string; updatedAt?: string }>(local: T[], remote: T[]) {
    const items = new Map<string, T>();
    remote.forEach((item) => {
        if (item.id) items.set(item.id, item);
    });
    local.forEach((item) => {
        if (!item.id) return;
        const current = items.get(item.id);
        if (!current || timeValue(item.updatedAt) >= timeValue(current.updatedAt)) items.set(item.id, item);
    });
    return Array.from(items.values()).sort((a, b) => timeValue(b.updatedAt) - timeValue(a.updatedAt));
}

function mergeRemoteSnapshot<T extends { id?: string; updatedAt?: string }>(local: T[], remote: T[], knownRemoteIds: Set<string>, pendingDeletedIds: Set<string>, knownRemoteVersions: Map<string, string>) {
    const remoteIds = new Set(remote.flatMap((item) => (item.id ? [item.id] : [])));
    const retainedLocal = local.filter((item) => {
        if (!item.id) return true;
        if (pendingDeletedIds.has(item.id)) return false;
        if (!knownRemoteIds.has(item.id) || remoteIds.has(item.id)) return true;
        const lastSyncedVersion = knownRemoteVersions.get(item.id);
        // 远端缺失只有在本地仍是上次同步版本时才可推断为远端删除；本地有更新时保留并重新推送，避免静默丢稿。
        return !sameVersion(lastSyncedVersion, item.updatedAt);
    });
    const retainedRemote = remote.filter((item) => !item.id || !pendingDeletedIds.has(item.id));
    return mergeById(retainedLocal, retainedRemote);
}

async function refreshRemoteStateBeforePush() {
    const snapshot = normalizeBackendMediaUrls(await getRemoteUserDataSnapshot());
    applyingRemoteState = true;
    try {
        const projects = mergeRemoteSnapshot(useCanvasStore.getState().projects, snapshot.projects, knownRemoteProjectIds, pendingDeletedProjectIds, knownRemoteProjectVersions);
        const assets = mergeRemoteSnapshot(useAssetStore.getState().assets, snapshot.assets, knownRemoteAssetIds, pendingDeletedAssetIds, knownRemoteAssetVersions);
        useCanvasStore.getState().replaceProjects(projects);
        useAssetStore.getState().replaceAssets(assets);
        remoteProjectVersions = versionMap(snapshot.projects);
        remoteAssetVersions = versionMap(snapshot.assets);
        rememberRemoteIds(snapshot.projects, snapshot.assets);
    } finally {
        applyingRemoteState = false;
    }
}

function mergePreparedItems<T extends { id: string; updatedAt?: string }>(latest: T[], source: T[], prepared: T[], uploaded: Map<string, string>) {
    const sourceById = new Map(source.map((item) => [item.id, item]));
    const preparedById = new Map(prepared.map((item) => [item.id, item]));
    return latest.map((item) => {
        const original = sourceById.get(item.id);
        const remoteReady = preparedById.get(item.id);
        if (!original || !remoteReady) return item;
        if (sameVersion(original.updatedAt, item.updatedAt)) return remoteReady;
        return replaceUploadedStorageReferences(item, uploaded);
    });
}

function replaceUploadedStorageReferences<T>(value: T, uploaded: Map<string, string>): T {
    if (!value || typeof value !== "object") return value;
    if (Array.isArray(value)) return value.map((item) => replaceUploadedStorageReferences(item, uploaded)) as T;
    const next = Object.fromEntries(Object.entries(value).map(([key, child]) => [key, replaceUploadedStorageReferences(child, uploaded)]));
    const storageKey = typeof next.storageKey === "string" ? next.storageKey : "";
    const replacement = uploaded.get(storageKey);
    return (replacement ? applyResourceReference(next, replacement) : next) as T;
}

function readKnownRemoteIds(userId: string) {
    const empty = {
        projects: new Set<string>(),
        assets: new Set<string>(),
        projectVersions: new Map<string, string>(),
        assetVersions: new Map<string, string>(),
        deletedProjects: new Set<string>(),
        deletedAssets: new Set<string>(),
    };
    if (typeof window === "undefined" || !window.localStorage || !userId) return empty;
    try {
        const value = JSON.parse(window.localStorage.getItem(`${REMOTE_KNOWN_IDS_STORAGE_PREFIX}${userId}`) || "{}") as {
            projects?: unknown;
            assets?: unknown;
            projectVersions?: unknown;
            assetVersions?: unknown;
            deletedProjects?: unknown;
            deletedAssets?: unknown;
        };
        return {
            projects: new Set(Array.isArray(value.projects) ? value.projects.filter((id): id is string => typeof id === "string") : []),
            assets: new Set(Array.isArray(value.assets) ? value.assets.filter((id): id is string => typeof id === "string") : []),
            projectVersions: stringMap(value.projectVersions),
            assetVersions: stringMap(value.assetVersions),
            deletedProjects: new Set(Array.isArray(value.deletedProjects) ? value.deletedProjects.filter((id): id is string => typeof id === "string") : []),
            deletedAssets: new Set(Array.isArray(value.deletedAssets) ? value.deletedAssets.filter((id): id is string => typeof id === "string") : []),
        };
    } catch {
        return empty;
    }
}

function rememberRemoteIds(projects: Array<{ id: string; updatedAt?: string }>, assets: Array<{ id: string; updatedAt?: string }>) {
    projects.forEach((item) => {
        knownRemoteProjectIds.add(item.id);
        knownRemoteProjectVersions.set(item.id, item.updatedAt || "");
    });
    assets.forEach((item) => {
        knownRemoteAssetIds.add(item.id);
        knownRemoteAssetVersions.set(item.id, item.updatedAt || "");
    });
    persistKnownRemoteIds();
}

function persistKnownRemoteIds() {
    if (typeof window === "undefined" || !window.localStorage || !activeRemoteUserId) return;
    try {
        window.localStorage.setItem(
            `${REMOTE_KNOWN_IDS_STORAGE_PREFIX}${activeRemoteUserId}`,
            JSON.stringify({
                projects: [...knownRemoteProjectIds],
                assets: [...knownRemoteAssetIds],
                projectVersions: Object.fromEntries(knownRemoteProjectVersions),
                assetVersions: Object.fromEntries(knownRemoteAssetVersions),
                deletedProjects: [...pendingDeletedProjectIds],
                deletedAssets: [...pendingDeletedAssetIds],
            }),
        );
    } catch {
        // 浏览器禁用本地存储时仍保留本次会话内的集合；服务端 tombstone 才能提供跨设备的严格原子保证。
    }
}

function rememberLocalDeletions<T extends { id: string }>(previous: T[], current: T[], knownRemoteIds: Set<string>, pendingDeletedIds: Set<string>) {
    if (!activeRemoteUserId || previous === current) return;
    const currentIds = new Set(current.map((item) => item.id));
    let changed = false;
    for (const item of previous) {
        if (!currentIds.has(item.id) && knownRemoteIds.has(item.id)) {
            pendingDeletedIds.add(item.id);
            changed = true;
        }
    }
    if (changed) persistKnownRemoteIds();
}

function versionMap(items: Array<{ id: string; updatedAt?: string }>) {
    return new Map<string, string>(items.map((item) => [item.id, item.updatedAt || ""]));
}

function missingIds<T extends { id: string }>(remote: Map<string, string>, local: T[]) {
    const localIds = new Set(local.map((item) => item.id));
    return Array.from(remote.keys()).filter((id) => !localIds.has(id));
}

function timeValue(value?: string) {
    const time = value ? Date.parse(value) : 0;
    return Number.isFinite(time) ? time : 0;
}

function sameVersion(remote?: string, local?: string) {
    return Boolean(remote && local) && timeValue(remote) === timeValue(local);
}

function isLocalStorageKey(value: string) {
    return LOCAL_STORAGE_KEY_PATTERN.test(value) && !resourceIdFromStorageKey(value);
}

function stringMap(value: unknown) {
    if (!value || typeof value !== "object" || Array.isArray(value)) return new Map<string, string>();
    return new Map(Object.entries(value).filter((entry): entry is [string, string] => typeof entry[1] === "string"));
}

function isImageStorageKey(value: string) {
    return value.startsWith("image:") || value.startsWith("generation-image:");
}

function numberValue(value: unknown) {
    const number = Number(value);
    return Number.isFinite(number) && number > 0 ? number : undefined;
}
