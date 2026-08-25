import { describe, expect, test } from "bun:test";

import {
    apiBaseURLFor,
    appPathFor,
    appRouterBasenameFor,
    normalizeAppBasePath,
    normalizeBackendMediaJsonFor,
    normalizeBackendMediaUrlFor,
    normalizeBackendMediaUrlsFor,
    stripAppBasePath,
} from "../src/lib/app-base-path-core";

describe("application base path", () => {
    test("normalizes root and subpath forms", () => {
        expect(normalizeAppBasePath()).toBe("/");
        expect(normalizeAppBasePath("/kai")).toBe("/kai/");
        expect(normalizeAppBasePath("kai/")).toBe("/kai/");
    });

    test("provides a React Router basename without a trailing slash", () => {
        expect(appRouterBasenameFor("/")).toBe("/");
        expect(appRouterBasenameFor("/kai/")).toBe("/kai");
    });

    test("prefixes application paths while preserving external URLs", () => {
        expect(appPathFor("create", "/")).toBe("/create");
        expect(appPathFor("/create", "/kai/")).toBe("/kai/create");
        expect(appPathFor("/kai/create", "/kai/")).toBe("/kai/create");
        expect(appPathFor("https://cdn.example.test/file.jpg", "/kai/")).toBe("https://cdn.example.test/file.jpg");
    });

    test("derives the API base from the application base unless explicitly configured", () => {
        expect(apiBaseURLFor(undefined, "/")).toBe("/api");
        expect(apiBaseURLFor(undefined, "/kai/")).toBe("/kai/api");
        expect(apiBaseURLFor("https://api.example.test/", "/kai/")).toBe("https://api.example.test");
    });

    test("strips the basename before route-module preloading", () => {
        expect(stripAppBasePath("/projects", "/")).toBe("/projects");
        expect(stripAppBasePath("/kai", "/kai/")).toBe("/");
        expect(stripAppBasePath("/kai/projects/one", "/kai/")).toBe("/projects/one");
    });

    test("prefixes only relative backend media URLs", () => {
        expect(normalizeBackendMediaUrlFor("/api/resources/one/file?download=1", "/kai/")).toBe("/kai/api/resources/one/file?download=1");
        expect(normalizeBackendMediaUrlFor("/api/admin/api-logs/log-one/media", "/kai/")).toBe("/kai/api/admin/api-logs/log-one/media");
        expect(normalizeBackendMediaUrlFor("api/resources/two/file", "/kai/")).toBe("/kai/api/resources/two/file");
        expect(normalizeBackendMediaUrlFor("/kai/api/resources/one/file", "/kai/")).toBe("/kai/api/resources/one/file");
        expect(normalizeBackendMediaUrlFor("https://cdn.example.test/api/file.mp4", "/kai/")).toBe("https://cdn.example.test/api/file.mp4");
        expect(normalizeBackendMediaUrlFor("data:image/png;base64,AAAA", "/kai/")).toBe("data:image/png;base64,AAAA");
        expect(normalizeBackendMediaUrlFor("文案中提到 /api/resources/one/file", "/kai/")).toBe("文案中提到 /api/resources/one/file");
    });

    test("normalizes nested canvas and task media without mutating the response", () => {
        const response = {
            project: { nodes: [{ metadata: { content: "/api/resources/image/file" } }] },
            task: { previewUrl: "/api/resources/video/file", prompt: "keep this text" },
        };
        const normalized = normalizeBackendMediaUrlsFor(response, "/kai/");

        expect(normalized).not.toBe(response);
        expect(normalized.project.nodes[0]?.metadata.content).toBe("/kai/api/resources/image/file");
        expect(normalized.task.previewUrl).toBe("/kai/api/resources/video/file");
        expect(response.project.nodes[0]?.metadata.content).toBe("/api/resources/image/file");
    });

    test("normalizes media URLs stored in task result JSON and preserves invalid JSON", () => {
        expect(normalizeBackendMediaJsonFor('{"video":{"dataUrl":"/api/resources/video/file"}}', "/kai/")).toBe('{"video":{"dataUrl":"/kai/api/resources/video/file"}}');
        expect(normalizeBackendMediaJsonFor("not-json", "/kai/")).toBe("not-json");
    });
});
