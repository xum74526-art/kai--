import { expect, test } from "bun:test";
import localforage from "localforage";

import { apiClient } from "../src/services/api/request";
import { installRemoteUserDataAutoSync, resetRemoteUserDataSync, syncRemoteUserData } from "../src/services/user-data-sync";
import { useAssetStore, type Asset } from "../src/stores/use-asset-store";
import { useCanvasStore } from "../src/stores/canvas/use-canvas-store";

type TimerCallback = () => void;

function imageAsset(id: string, title: string, dataUrl = "https://example.invalid/image.png"): Asset {
    return {
        id,
        kind: "image",
        title,
        coverUrl: dataUrl,
        tags: [],
        metadata: {},
        data: { dataUrl, width: 1, height: 1, bytes: 1, mimeType: "image/png" },
        createdAt: "2026-08-20T00:00:00.000Z",
        updatedAt: "2026-08-20T00:00:00.000Z",
    };
}

function resource(id: string) {
    return {
        id,
        userId: "sync-user",
        kind: "image",
        status: "ready",
        provider: "local",
        endpoint: "",
        bucket: "",
        objectKey: id,
        publicUrl: "",
        mimeType: "image/png",
        size: 1,
        createdAt: "2026-08-26T00:00:00.000Z",
        updatedAt: "2026-08-26T00:00:00.000Z",
    };
}

async function withSyncEnvironment(run: (environment: { localStorageValues: Map<string, string>; timers: Map<number, TimerCallback> }) => Promise<void>) {
    const originalWindow = (globalThis as { window?: unknown }).window;
    const originalGetItem = localforage.getItem.bind(localforage);
    const originalSetItem = localforage.setItem.bind(localforage);
    const originalRemoveItem = localforage.removeItem.bind(localforage);
    const previousAdapter = apiClient.defaults.adapter;
    const previousAssets = useAssetStore.getState().assets;
    const previousProjects = useCanvasStore.getState().projects;
    const indexedValues = new Map<string, string>();
    const localStorageValues = new Map<string, string>();
    const timers = new Map<number, TimerCallback>();
    let timerId = 0;
    Object.defineProperty(globalThis, "window", {
        configurable: true,
        value: {
            localStorage: {
                getItem: (key: string) => localStorageValues.get(key) ?? null,
                setItem: (key: string, value: string) => localStorageValues.set(key, value),
                removeItem: (key: string) => localStorageValues.delete(key),
            },
            setTimeout: (callback: TimerCallback) => {
                const id = ++timerId;
                timers.set(id, callback);
                return id;
            },
            clearTimeout: (id: number) => timers.delete(id),
        },
    });
    localforage.getItem = (async (key: string) => indexedValues.get(key) ?? null) as typeof localforage.getItem;
    localforage.setItem = (async (key: string, value: string) => {
        indexedValues.set(key, value);
        return value;
    }) as typeof localforage.setItem;
    localforage.removeItem = (async (key: string) => {
        indexedValues.delete(key);
    }) as typeof localforage.removeItem;
    installRemoteUserDataAutoSync();
    try {
        await run({ localStorageValues, timers });
    } finally {
        resetRemoteUserDataSync();
        useAssetStore.getState().replaceAssets(previousAssets);
        useCanvasStore.getState().replaceProjects(previousProjects);
        apiClient.defaults.adapter = previousAdapter;
        localforage.getItem = originalGetItem;
        localforage.setItem = originalSetItem;
        localforage.removeItem = originalRemoveItem;
        if (originalWindow === undefined) delete (globalThis as { window?: unknown }).window;
        else Object.defineProperty(globalThis, "window", { configurable: true, value: originalWindow });
    }
}

test.serial("a previously synced item deleted remotely is not resurrected by a stale device", async () => {
    await withSyncEnvironment(async () => {
        let remoteAssets: Asset[] = [{ ...imageAsset("shared", "remote"), data: { ...imageAsset("shared", "remote").data, storageKey: "resource:shared-resource" } }];
        const upserts: string[] = [];
        apiClient.defaults.adapter = async (config) => {
            const url = String(config.url || "");
            if (url.includes("user-data/snapshot")) return { data: { code: 0, data: { projects: [], assets: remoteAssets }, msg: "" }, status: 200, statusText: "OK", headers: {}, config };
            if (config.method === "put" && url.includes("/assets/")) {
                upserts.push(url);
                return { data: { code: 0, data: { asset: { id: "shared", title: "remote", createdAt: "", updatedAt: "" } }, msg: "" }, status: 200, statusText: "OK", headers: {}, config };
            }
            return { data: { code: 0, data: { id: "shared" }, msg: "" }, status: 200, statusText: "OK", headers: {}, config };
        };
        useAssetStore.getState().replaceAssets([]);
        useCanvasStore.getState().replaceProjects([]);
        await syncRemoteUserData("stale-device-user");
        expect(useAssetStore.getState().assets.map((item) => item.id)).toEqual(["shared"]);

        resetRemoteUserDataSync();
        remoteAssets = [];
        await syncRemoteUserData("stale-device-user");
        expect(useAssetStore.getState().assets).toEqual([]);
        expect(upserts).toEqual([]);
    });
});

test.serial("a local edit made after the last sync survives a concurrent remote deletion", async () => {
    await withSyncEnvironment(async () => {
        const synced = {
            ...imageAsset("conflict", "last synced"),
            data: { ...imageAsset("conflict", "last synced").data, storageKey: "resource:conflict-resource" },
        };
        let remoteAssets: Asset[] = [synced];
        const upsertedTitles: string[] = [];
        apiClient.defaults.adapter = async (config) => {
            const url = String(config.url || "");
            if (url.includes("user-data/snapshot")) {
                return { data: { code: 0, data: { projects: [], assets: remoteAssets }, msg: "" }, status: 200, statusText: "OK", headers: {}, config };
            }
            if (config.method === "put" && url.includes("/assets/")) {
                const payload = JSON.parse(String(config.data || "{}")) as { asset: Asset };
                upsertedTitles.push(payload.asset.title);
                remoteAssets = [payload.asset];
                return { data: { code: 0, data: { asset: payload.asset }, msg: "" }, status: 200, statusText: "OK", headers: {}, config };
            }
            throw new Error(`unexpected request: ${config.method} ${url}`);
        };
        useAssetStore.getState().replaceAssets([]);
        useCanvasStore.getState().replaceProjects([]);
        await syncRemoteUserData("conflict-user");

        resetRemoteUserDataSync();
        remoteAssets = [];
        useAssetStore.getState().replaceAssets([{ ...synced, title: "local newer edit", updatedAt: "2026-08-21T00:00:00.000Z" }]);
        await syncRemoteUserData("conflict-user");

        expect(useAssetStore.getState().assets[0]?.title).toBe("local newer edit");
        expect(upsertedTitles.at(-1)).toBe("local newer edit");
    });
});

test.serial("an item deleted while its first upsert is in flight is removed remotely", async () => {
    await withSyncEnvironment(async () => {
        let remoteAssets: Asset[] = [];
        let releaseUpsert!: () => void;
        const upsertReleased = new Promise<void>((resolve) => {
            releaseUpsert = resolve;
        });
        let markUpsertStarted!: () => void;
        const upsertStarted = new Promise<void>((resolve) => {
            markUpsertStarted = resolve;
        });
        let deleteCount = 0;
        apiClient.defaults.adapter = async (config) => {
            const url = String(config.url || "");
            if (url.includes("user-data/snapshot")) {
                return { data: { code: 0, data: { projects: [], assets: remoteAssets }, msg: "" }, status: 200, statusText: "OK", headers: {}, config };
            }
            if (config.method === "put" && url.includes("/assets/")) {
                const payload = JSON.parse(String(config.data || "{}")) as { asset: Asset };
                markUpsertStarted();
                await upsertReleased;
                remoteAssets = [payload.asset];
                return { data: { code: 0, data: { asset: payload.asset }, msg: "" }, status: 200, statusText: "OK", headers: {}, config };
            }
            if (config.method === "delete" && url.includes("/assets/")) {
                deleteCount += 1;
                remoteAssets = [];
                return { data: { code: 0, data: { id: "in-flight" }, msg: "" }, status: 200, statusText: "OK", headers: {}, config };
            }
            throw new Error(`unexpected request: ${config.method} ${url}`);
        };
        const local = {
            ...imageAsset("in-flight", "delete during upload"),
            data: { ...imageAsset("in-flight", "delete during upload").data, storageKey: "resource:in-flight-resource" },
        };
        useAssetStore.getState().replaceAssets([local]);
        useCanvasStore.getState().replaceProjects([]);
        const syncing = syncRemoteUserData("in-flight-user");
        await upsertStarted;
        useAssetStore.getState().removeAsset("in-flight");
        releaseUpsert();
        await syncing;

        expect(useAssetStore.getState().assets).toEqual([]);
        expect(remoteAssets).toEqual([]);
        expect(deleteCount).toBe(1);
    });
});

test.serial("an edit made while media uploads is preserved and sent in the next drained batch", async () => {
    await withSyncEnvironment(async () => {
        let remoteAssets: Asset[] = [];
        let releaseFirstUpload!: () => void;
        const firstUploadReleased = new Promise<void>((resolve) => {
            releaseFirstUpload = resolve;
        });
        let firstUploadStarted!: () => void;
        const uploadStarted = new Promise<void>((resolve) => {
            firstUploadStarted = resolve;
        });
        let uploadCount = 0;
        const upsertedTitles: string[] = [];
        apiClient.defaults.adapter = async (config) => {
            const url = String(config.url || "");
            if (url.includes("user-data/snapshot")) return { data: { code: 0, data: { projects: [], assets: remoteAssets }, msg: "" }, status: 200, statusText: "OK", headers: {}, config };
            if (url.endsWith("/resources")) {
                uploadCount += 1;
                if (uploadCount === 1) {
                    firstUploadStarted();
                    await firstUploadReleased;
                }
                return { data: { code: 0, data: { resource: resource(`uploaded-${uploadCount}`) }, msg: "" }, status: 200, statusText: "OK", headers: {}, config };
            }
            if (config.method === "put" && url.includes("/assets/")) {
                const payload = JSON.parse(String(config.data || "{}")) as { asset: Asset };
                upsertedTitles.push(payload.asset.title);
                remoteAssets = [payload.asset];
                return { data: { code: 0, data: { asset: payload.asset }, msg: "" }, status: 200, statusText: "OK", headers: {}, config };
            }
            throw new Error(`unexpected request: ${config.method} ${url}`);
        };
        const source = imageAsset("editing", "before upload", "data:image/png;base64,iVBORw0KGgo=");
        useAssetStore.getState().replaceAssets([source]);
        useCanvasStore.getState().replaceProjects([]);
        const syncing = syncRemoteUserData("editing-user");
        await uploadStarted;
        useAssetStore.getState().updateAsset("editing", { title: "edited during upload" });
        releaseFirstUpload();
        await syncing;
        expect(useAssetStore.getState().assets[0]?.title).toBe("edited during upload");
        expect(upsertedTitles.at(-1)).toBe("edited during upload");
    });
});

test.serial("failed media upload remains dirty and succeeds on the scheduled retry", async () => {
    await withSyncEnvironment(async ({ timers }) => {
        let failUpload = true;
        let remoteAssets: Asset[] = [];
        let upsertCount = 0;
        apiClient.defaults.adapter = async (config) => {
            const url = String(config.url || "");
            if (url.includes("user-data/snapshot")) return { data: { code: 0, data: { projects: [], assets: remoteAssets }, msg: "" }, status: 200, statusText: "OK", headers: {}, config };
            if (url.endsWith("/resources")) {
                if (failUpload) throw new Error("upload unavailable");
                return { data: { code: 0, data: { resource: resource("retry-resource") }, msg: "" }, status: 200, statusText: "OK", headers: {}, config };
            }
            if (config.method === "put" && url.includes("/assets/")) {
                const payload = JSON.parse(String(config.data || "{}")) as { asset: Asset };
                remoteAssets = [payload.asset];
                upsertCount += 1;
                return { data: { code: 0, data: { asset: payload.asset }, msg: "" }, status: 200, statusText: "OK", headers: {}, config };
            }
            throw new Error(`unexpected request: ${config.method} ${url}`);
        };
        useAssetStore.getState().replaceAssets([imageAsset("retry", "retry", "data:image/png;base64,iVBORw0KGgo=")]);
        useCanvasStore.getState().replaceProjects([]);
        await syncRemoteUserData("retry-user");
        expect(upsertCount).toBe(0);
        expect(timers.size).toBeGreaterThan(0);

        failUpload = false;
        const retry = [...timers.values()].at(-1);
        if (!retry) throw new Error("retry timer was not scheduled");
        retry();
        for (let index = 0; index < 20 && upsertCount === 0; index += 1) await new Promise<void>((resolve) => setTimeout(resolve, 0));
        expect(upsertCount).toBe(1);
    });
});
