import { nanoid } from "nanoid";

import { getActiveUserScope } from "@/lib/user-scope";

export type ImportedStorageReference = {
    storageKey: string;
    url: string;
};

const SUPPORTED_PREFIXES = new Set(["image", "generation-image", "video", "generation-video", "audio", "generation-audio", "file", "model", "video-reference", "audio-reference"]);

export function isImageStorageReference(storageKey: string, mimeType = "") {
    return storageKey.startsWith("image:") || storageKey.startsWith("generation-image:") || mimeType.startsWith("image/");
}

export function createImportedStorageKey(storageKey: string, mimeType = "") {
    const archivedPrefix = storageKey.split(":", 1)[0];
    const prefix = SUPPORTED_PREFIXES.has(archivedPrefix) ? archivedPrefix : mimeType.startsWith("image/") ? "image" : mimeType.startsWith("video/") ? "video" : mimeType.startsWith("audio/") ? "audio" : "file";
    return `${prefix}:${getActiveUserScope()}:import-${nanoid()}`;
}

/**
 * 导入包不能复用包内 storageKey，否则会覆盖当前账号已有 Blob。
 * 同时替换与旧媒体 URL 相同的封面/预览字段，避免导入后继续引用失效的 blob: URL。
 */
export function remapImportedStorageReferences<T>(value: T, references: Map<string, ImportedStorageReference>): T {
    const archivedUrls = collectArchivedUrls(value, references);
    return remapValue(value, references, archivedUrls);
}

function collectArchivedUrls(value: unknown, references: Map<string, ImportedStorageReference>, urls = new Map<string, string>()) {
    if (!value || typeof value !== "object") return urls;
    if (Array.isArray(value)) {
        value.forEach((item) => collectArchivedUrls(item, references, urls));
        return urls;
    }
    const payload = value as Record<string, unknown>;
    const storageKey = typeof payload.storageKey === "string" ? payload.storageKey : "";
    const reference = references.get(storageKey);
    if (reference) {
        for (const key of ["content", "dataUrl", "url", "coverUrl"]) {
            const archivedUrl = payload[key];
            if (typeof archivedUrl === "string" && archivedUrl) urls.set(archivedUrl, reference.url);
        }
    }
    Object.values(payload).forEach((item) => collectArchivedUrls(item, references, urls));
    return urls;
}

function remapValue<T>(value: T, references: Map<string, ImportedStorageReference>, archivedUrls: Map<string, string>): T {
    if (typeof value === "string") return (archivedUrls.get(value) ?? value) as T;
    if (!value || typeof value !== "object") return value;
    if (Array.isArray(value)) return value.map((item) => remapValue(item, references, archivedUrls)) as T;
    const payload = Object.fromEntries(Object.entries(value).map(([key, child]) => [key, remapValue(child, references, archivedUrls)]));
    const storageKey = typeof payload.storageKey === "string" ? payload.storageKey : "";
    const reference = references.get(storageKey);
    if (!reference) return payload as T;
    payload.storageKey = reference.storageKey;
    for (const key of ["content", "dataUrl", "url", "coverUrl"]) {
        if (typeof payload[key] === "string") payload[key] = reference.url;
    }
    return payload as T;
}
