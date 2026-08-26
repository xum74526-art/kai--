import { saveAs } from "file-saver";

import { createZip, readZip } from "@/lib/zip";
import { deleteStoredMedia, getMediaBlob, setMediaBlob } from "@/services/file-storage";
import { deleteStoredImages, getImageBlob, setImageBlob } from "@/services/image-storage";
import type { Asset } from "@/stores/use-asset-store";
import { createImportedStorageKey, isImageStorageReference, remapImportedStorageReferences, type ImportedStorageReference } from "@/lib/imported-storage";

type AssetExportFile = {
    app: "infinite-canvas";
    version: 1;
    exportedAt: string;
    assets: Asset[];
    files: AssetExportItem[];
};

type AssetExportItem = {
    storageKey: string;
    path: string;
    mimeType: string;
    bytes: number;
};

export async function exportAssets(assets: Asset[]) {
    const files: AssetExportItem[] = [];
    const zipFiles: { name: string; data: BlobPart }[] = [];

    await Promise.all(
        assets.map(async (asset) => {
            if (asset.kind !== "image" && asset.kind !== "video" && asset.kind !== "audio" && asset.kind !== "model") return;
            const storageKey = asset.data.storageKey;
            if (!storageKey) return;
            const blob = isImageStorageReference(storageKey, asset.data.mimeType) ? await getImageBlob(storageKey) : await getMediaBlob(storageKey);
            if (!blob) throw new Error(`素材“${asset.title}”引用的本地文件已丢失：${storageKey}`);
            const path = `files/${safeFileName(storageKey)}.${fileExtension(blob.type, asset.kind)}`;
            files.push({ storageKey, path, mimeType: blob.type || asset.data.mimeType, bytes: blob.size });
            zipFiles.push({ name: path, data: blob });
        }),
    );

    const data: AssetExportFile = { app: "infinite-canvas", version: 1, exportedAt: new Date().toISOString(), assets, files };
    const zip = await createZip([{ name: "assets.json", data: JSON.stringify(data, null, 2) }, ...zipFiles]);
    saveAs(zip, "我的素材.zip");
}

export async function readAssetPackage(file: File) {
    const zip = await readZip(file);
    const assetFile = zip.get("assets.json");
    if (!assetFile) throw new Error("missing assets.json");
    const data = JSON.parse(await assetFile.text()) as AssetExportFile;
    if (!Array.isArray(data.assets) || !Array.isArray(data.files)) throw new Error("素材压缩包结构无效");
    for (const item of data.files) {
        if (!zip.get(item.path)) throw new Error(`素材压缩包缺少文件：${item.path}`);
    }
    const references = new Map<string, ImportedStorageReference>();
    try {
        for (const item of data.files) {
            if (references.has(item.storageKey)) continue;
            const blob = zip.get(item.path)!;
            const typedBlob = blob.type ? blob : blob.slice(0, blob.size, item.mimeType);
            const storageKey = createImportedStorageKey(item.storageKey, item.mimeType);
            const url = isImageStorageReference(item.storageKey, item.mimeType) ? await setImageBlob(storageKey, typedBlob) : await setMediaBlob(storageKey, typedBlob);
            references.set(item.storageKey, { storageKey, url });
        }
        return remapImportedStorageReferences(data.assets, references);
    } catch (error) {
        await cleanupImportedAssetFilesFromReferences(references.values());
        throw error;
    }
}

export async function cleanupImportedAssetFiles(assets: Asset[]) {
    const references = assets.flatMap((asset) => {
        if (asset.kind !== "image" && asset.kind !== "video" && asset.kind !== "audio" && asset.kind !== "model") return [];
        const storageKey = asset.data.storageKey;
        return storageKey ? [{ storageKey, mimeType: asset.data.mimeType }] : [];
    });
    await cleanupImportedAssetFilesFromReferences(references);
}

async function cleanupImportedAssetFilesFromReferences(references: Iterable<{ storageKey: string; mimeType?: string }>) {
    const imageKeys: string[] = [];
    const mediaKeys: string[] = [];
    for (const reference of references) {
        (isImageStorageReference(reference.storageKey, reference.mimeType) ? imageKeys : mediaKeys).push(reference.storageKey);
    }
    await Promise.all([deleteStoredImages(imageKeys), deleteStoredMedia(mediaKeys)]);
}

function safeFileName(value: string) {
    return value.replace(/[\\/:*?"<>|]/g, "_");
}

function fileExtension(mimeType: string, kind: Asset["kind"]) {
    if (mimeType.includes("png")) return "png";
    if (mimeType.includes("jpeg")) return "jpg";
    if (mimeType.includes("webp")) return "webp";
    if (mimeType.includes("gif")) return "gif";
    if (mimeType.includes("mp4")) return "mp4";
    if (mimeType.includes("webm")) return "webm";
    if (mimeType.includes("mpeg")) return "mp3";
    if (mimeType.includes("wav")) return "wav";
    if (mimeType.includes("gltf-binary")) return "glb";
    if (mimeType.includes("gltf+json") || mimeType.includes("json")) return "gltf";
    return kind === "image" ? "png" : "bin";
}
