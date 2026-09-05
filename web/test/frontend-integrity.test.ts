import { expect, test } from "bun:test";

import { apiClient } from "../src/services/api/request";
import { getAuthSession, logout, register, type LocalUser } from "../src/services/api/auth";
import { createImportedStorageKey, remapImportedStorageReferences } from "../src/lib/imported-storage";
import { persistSystemStyleLibraryIds, readSystemStyleLibraryIds } from "../src/lib/canvas/canvas-style-library-storage";
import { testChannelModelConnection } from "../src/lib/model-connection-test";
import { setActiveUserScope } from "../src/lib/user-scope";
import { uploadMediaFile } from "../src/services/file-storage";
import { useCanvasStore } from "../src/stores/canvas/use-canvas-store";
import { createModelChannel } from "../src/stores/use-config-store";
import type { TimelineProject } from "../src/types/timeline";

test.serial("register invalidates a cached guest session", async () => {
    const previousAdapter = apiClient.defaults.adapter;
    let currentUser: LocalUser | null = null;
    let sessionRequests = 0;
    const user: LocalUser = {
        id: "registered-user",
        username: "registered-user",
        displayName: "Registered User",
        role: "user",
        status: "active",
        createdAt: "2026-08-26T00:00:00.000Z",
        updatedAt: "2026-08-26T00:00:00.000Z",
    };
    apiClient.defaults.adapter = async (config) => {
        const url = String(config.url || "");
        if (url.endsWith("/auth/logout")) currentUser = null;
        if (url.endsWith("/auth/register")) currentUser = user;
        if (url.endsWith("/auth/session")) sessionRequests += 1;
        const data = url.endsWith("/auth/session") ? { user: currentUser } : url.endsWith("/auth/register") ? { user } : { ok: true };
        return { data: { code: 0, data, msg: "" }, status: 200, statusText: "OK", headers: {}, config };
    };
    try {
        await logout();
        expect((await getAuthSession()).user).toBeNull();
        await register({ username: user.username, password: "password-for-test" });
        expect((await getAuthSession()).user?.id).toBe(user.id);
        expect(sessionRequests).toBe(2);
    } finally {
        apiClient.defaults.adapter = previousAdapter;
    }
});

test.serial("image model connection tests use a queued backend task instead of a long custom relay request", async () => {
    const previousAdapter = apiClient.defaults.adapter;
    const previousWindow = (globalThis as { window?: unknown }).window;
    const calls: string[] = [];
    const taskId = "image-model-test-task";
    const createdAt = "2026-08-28T00:00:00.000Z";
    Object.defineProperty(globalThis, "window", {
        configurable: true,
        value: { dispatchEvent: () => true, location: { hostname: "localhost" } },
    });
    apiClient.defaults.adapter = async (config) => {
        const method = String(config.method || "get").toLowerCase();
        const url = String(config.url || "");
        calls.push(`${method}:${url}`);
        const task = {
            id: taskId,
            type: "canvas_image",
            status: method === "post" ? "queued" : "succeeded",
            prompt: "A simple gray circle on a white background.",
            operation: "image",
            model: "user-bailian::qwen-image-3.0-pro",
            attempts: 1,
            createdAt,
            updatedAt: createdAt,
            ...(method === "get" ? { resultJson: JSON.stringify({ mode: "image", images: [{ dataUrl: "data:image/png;base64,AA==" }] }) } : {}),
        };
        if (method === "post") {
            const body = JSON.parse(String(config.data)) as { input?: { metadata?: { source?: string }; config?: { interfaceType?: string } } };
            expect(body.input?.metadata?.source).toBe("model-connection-test");
            expect(body.input?.config?.interfaceType).toBe("bailian-image");
        }
        return { data: { code: 0, data: task, msg: "" }, status: 200, statusText: "OK", headers: {}, config };
    };
    const channel = createModelChannel({
        id: "user-bailian",
        name: "Bailian",
        baseUrl: "https://example.invalid/compatible-mode/v1",
        apiKey: "synthetic-test-key",
        models: ["qwen-image-3.0-pro"],
        modelCosts: [{ model: "qwen-image-3.0-pro", capability: "image", protocol: "bailian-image", billingMode: "fixed_request", unitPriceMicrocredits: 0 }],
    });
    let queuedTaskId = "";
    try {
        const detail = await testChannelModelConnection(channel, "qwen-image-3.0-pro", "image", "bailian-image", { onTaskCreated: (id) => (queuedTaskId = id) });
        expect(detail).toContain("图片生成正常");
        expect(queuedTaskId).toBe(taskId);
        expect(calls).toEqual(["post:/tasks", `get:/tasks/${taskId}`]);
        expect(calls.some((call) => call.includes("/ai/custom"))).toBe(false);
    } finally {
        apiClient.defaults.adapter = previousAdapter;
        if (previousWindow === undefined) delete (globalThis as { window?: unknown }).window;
        else Object.defineProperty(globalThis, "window", { configurable: true, value: previousWindow });
    }
});

test("registration UI never promises an automatic administrator or skips email verification for the first account", async () => {
    const source = await Bun.file(new URL("../src/pages/auth/register.tsx", import.meta.url)).text();
    expect(source).not.toContain("首个账号自动成为管理员");
    expect(source).not.toContain("管理员账号已创建");
    expect(source).toContain('message.success("普通用户账号注册成功")');
    expect(source).toContain("const requireCode = Boolean(settings?.emailCodeRequired)");
});

test("imported storage references receive a scoped key and replace stale nested blob URLs", () => {
    const originalWindow = (globalThis as { window?: unknown }).window;
    const values = new Map<string, string>([["infinite-canvas:active-user-scope", "import-user"]]);
    Object.defineProperty(globalThis, "window", {
        configurable: true,
        value: { localStorage: { getItem: (key: string) => values.get(key) ?? null, setItem: (key: string, value: string) => values.set(key, value) } },
    });
    try {
        const nextKey = createImportedStorageKey("generation-image:old-user:key", "image/png");
        expect(nextKey.startsWith("generation-image:import-user:import-")).toBe(true);
        const value = {
            coverUrl: "blob:archived-preview",
            data: { storageKey: "generation-image:old-user:key", dataUrl: "blob:archived-preview" },
        };
        const remapped = remapImportedStorageReferences(value, new Map([["generation-image:old-user:key", { storageKey: nextKey, url: "blob:new-preview" }]]));
        expect(remapped).toEqual({ coverUrl: "blob:new-preview", data: { storageKey: nextKey, dataUrl: "blob:new-preview" } });
    } finally {
        if (originalWindow === undefined) delete (globalThis as { window?: unknown }).window;
        else Object.defineProperty(globalThis, "window", { configurable: true, value: originalWindow });
    }
});

test("system style favorites and recent usage stay in the active user scope", () => {
    const originalWindow = (globalThis as { window?: unknown }).window;
    const values = new Map<string, string>();
    Object.defineProperty(globalThis, "window", {
        configurable: true,
        value: {
            localStorage: {
                getItem: (key: string) => values.get(key) ?? null,
                setItem: (key: string, value: string) => values.set(key, value),
                removeItem: (key: string) => values.delete(key),
            },
        },
    });
    try {
        setActiveUserScope("style-user-a");
        persistSystemStyleLibraryIds("favorites", ["noir"]);
        persistSystemStyleLibraryIds("recent", ["period"]);

        setActiveUserScope("style-user-b");
        expect(readSystemStyleLibraryIds("favorites")).toEqual([]);
        expect(readSystemStyleLibraryIds("recent")).toEqual([]);
        persistSystemStyleLibraryIds("favorites", ["animation"]);

        setActiveUserScope("style-user-a");
        expect(readSystemStyleLibraryIds("favorites")).toEqual(["noir"]);
        expect(readSystemStyleLibraryIds("recent")).toEqual(["period"]);

        setActiveUserScope(null);
        expect(readSystemStyleLibraryIds("favorites")).toEqual([]);
    } finally {
        if (originalWindow === undefined) delete (globalThis as { window?: unknown }).window;
        else Object.defineProperty(globalThis, "window", { configurable: true, value: originalWindow });
    }
});

test("canvas import preserves timeline data", () => {
    const previous = useCanvasStore.getState().projects;
    const timeline: TimelineProject = { version: 1, durationMs: 1_000, tracks: [], clips: [], updatedAt: "2026-08-26T00:00:00.000Z" };
    try {
        const id = useCanvasStore.getState().importProject({ title: "timeline import", timeline });
        expect(useCanvasStore.getState().projects.find((item) => item.id === id)?.timeline).toEqual(timeline);
    } finally {
        useCanvasStore.getState().replaceProjects(previous);
    }
});

test("media URL upload rejects HTTP error bodies instead of storing them as blobs", async () => {
    const previousFetch = globalThis.fetch;
    globalThis.fetch = (async () => new Response("not found", { status: 404, headers: { "content-type": "text/html" } })) as typeof fetch;
    try {
        await expect(uploadMediaFile("https://example.invalid/missing.mp4")).rejects.toThrow("HTTP 404");
    } finally {
        globalThis.fetch = previousFetch;
    }
});
