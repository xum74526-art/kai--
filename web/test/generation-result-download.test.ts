import { afterEach, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

import { fetchGenerationResultBlob, projectGenerationTaskResult } from "../src/services/project-asset-sync";

const originalFetch = globalThis.fetch;

afterEach(() => {
    globalThis.fetch = originalFetch;
});

function mockFetch(response: Response) {
    globalThis.fetch = (async () => response) as typeof fetch;
}

test("generation result download rejects failed HTTP responses before caching", async () => {
    mockFetch(new Response("expired", { status: 403, headers: { "content-type": "text/html" } }));

    await expect(fetchGenerationResultBlob("https://example.test/expired", "image")).rejects.toThrow("HTTP 403");
});

test("generation result download rejects an upstream error page masquerading as media", async () => {
    mockFetch(new Response("<html><body>expired</body></html>", { headers: { "content-type": "text/html; charset=utf-8" } }));

    await expect(fetchGenerationResultBlob("https://example.test/error-page", "video")).rejects.toThrow("非视频内容");
});

test("generation result download rejects an untyped signed-URL error body", async () => {
    mockFetch(new Response(new Blob(["AccessDenied"])));

    await expect(fetchGenerationResultBlob("https://example.test/access-denied", "image")).rejects.toThrow("错误页面");
});

test("generation result download accepts binary signed-download responses", async () => {
    mockFetch(new Response(new Uint8Array([1, 2, 3]), { headers: { "content-type": "application/octet-stream" } }));

    const blob = await fetchGenerationResultBlob("https://example.test/output", "audio");
    expect(blob.size).toBe(3);
});

test("generation task projection retains a distinct output index for every image", () => {
    const task = projectGenerationTaskResult(
        {
            id: "task-multi-image",
            type: "image",
            status: "succeeded",
            prompt: "test",
            attempts: 1,
            createdAt: "2026-09-04T00:00:00.000Z",
            updatedAt: "2026-09-04T00:00:00.000Z",
        },
        { mode: "image", images: [{ dataUrl: "https://example.test/0" }, { dataUrl: "https://example.test/1" }] },
    );

    expect(task.outputs?.map((output) => output.outputIndex)).toEqual([0, 1]);
});

test("creation page passes each task's complete image batch into materialization", () => {
    const source = readFileSync(resolve(import.meta.dir, "../src/pages/create/index.tsx"), "utf8");

    expect(source).toContain("const generatedImageBatches");
    expect(source).toContain('result: { mode: "image", images }');
    expect(source).toContain("urls.length !== images.length");
});
