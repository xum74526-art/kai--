import { afterEach, describe, expect, test } from "bun:test";
import axios from "axios";

import { consumeChatCompletionStreamText, consumeGeminiStreamText, consumeResponseStreamText, parseChatCompletionPayload, parseGeminiToolResponse, parseImagePayload, parseToolResponse } from "../src/services/api/image-response";
import { normalizeQuality, resolveRequestSize, validateImageSize } from "../src/services/api/image-validation";
import { bailianImageApiUrl, buildBailianImagePayload, parseBailianImagePayload } from "../src/services/api/bailian-image";
import { defaultImageCapabilityConfig } from "../src/lib/model-capabilities";
import { requestGeneration } from "../src/services/api/image";
import { createModelChannel, defaultConfig } from "../src/stores/use-config-store";

const originalAxiosPost = axios.post;

afterEach(() => {
    axios.post = originalAxiosPost;
});

describe("image api contracts", () => {
    test("规范化质量和尺寸，拒绝超出边界的图像", () => {
        expect(normalizeQuality("1k")).toBe("low");
        expect(normalizeQuality("2k")).toBe("medium");
        expect(normalizeQuality("4k")).toBe("high");
        expect(resolveRequestSize("medium", "16:9")).toMatch(/^\d+x\d+$/);
        expect(resolveRequestSize("medium", "9:16")).toMatch(/^\d+x\d+$/);
        expect(() => validateImageSize(1025, 1024)).toThrow("16 的倍数");
        expect(() => validateImageSize(3072, 1008)).toThrow("宽高比");
        expect(() => validateImageSize(3840, 2176)).toThrow("总像素");
    });

    test("解包 b64_json、url 和业务错误", () => {
        expect(parseImagePayload({ data: [{ b64_json: "abc" }, { url: "https://example.com/image.png" }] })).toEqual([
            { id: expect.any(String), dataUrl: "data:image/png;base64,abc" },
            { id: expect.any(String), dataUrl: "https://example.com/image.png" },
        ]);
        expect(() => parseImagePayload({ data: [] })).toThrow("接口没有返回图片");
        expect(() => parseImagePayload({ code: 1001, msg: "额度不足" })).toThrow("额度不足");
    });

    test("构造并解析百炼原生图片协议", () => {
        expect(bailianImageApiUrl("https://workspace.cn-beijing.maas.aliyuncs.com/compatible-mode/v1")).toBe(
            "https://workspace.cn-beijing.maas.aliyuncs.com/api/v1/services/aigc/multimodal-generation/generation",
        );
        expect(bailianImageApiUrl("/api/ai/system/bailian")).toBe("/api/ai/system/bailian/api/v1/services/aigc/multimodal-generation/generation");
        expect(
            buildBailianImagePayload({
                model: "qwen-image-3.0-pro",
                prompt: "生成商品主图",
                images: ["data:image/png;base64,abc"],
                count: 2,
                size: "1024x1536",
            }),
        ).toEqual({
            model: "qwen-image-3.0-pro",
            input: { messages: [{ role: "user", content: [{ image: "data:image/png;base64,abc" }, { text: "生成商品主图" }] }] },
            parameters: { n: 2, watermark: false, size: "1024*1536" },
        });
        expect(() => buildBailianImagePayload({ model: "wan2.7-image-pro", prompt: "编辑图片", images: ["https://example.com/reference.png"], count: 1, size: "4K" })).toThrow("最高支持 2K");
        expect(parseBailianImagePayload({ output: { choices: [{ message: { content: [{ image: "https://example.com/result.png" }] } }] } })).toEqual([
            { id: expect.any(String), dataUrl: "https://example.com/result.png" },
        ]);
        expect(() => parseBailianImagePayload({ code: "InvalidParameter", message: "尺寸错误" })).toThrow("尺寸错误");
    });

    test("按百炼模型区分图片能力默认值", () => {
        const wanPro = defaultImageCapabilityConfig("bailian-image", "wan2.7-image-pro");
        expect(wanPro.references).toMatchObject({ maxImages: 9, maxImageBytes: 20 * 1024 * 1024, maskSupported: false });
        expect(wanPro.size).toMatchObject({ values: ["1K", "2K", "4K"], default: "2K", allowCustom: true });
        expect(wanPro.maxOutputs).toBe(4);
        expect(buildBailianImagePayload({ model: "wan2.7-image-pro", prompt: "生成图片", images: [], count: 1, size: "2048x2048" }).parameters.size).toBe("2048*2048");

        const qwen = defaultImageCapabilityConfig("bailian-image", "qwen-image-3.0-pro");
        expect(qwen.references).toMatchObject({ maxImages: 3, maxImageBytes: 10 * 1024 * 1024, maskSupported: false });
        expect(qwen.size.values).toContain("2048x2048");
        expect(qwen.maxOutputs).toBe(6);
    });

    test("百炼图片生成通过自定义渠道中转到原生端点", async () => {
        let requestUrl = "";
        let requestBody: unknown;
        let requestHeaders: Record<string, string> = {};
        axios.post = (async (url: string, body: unknown, config?: { headers?: Record<string, string> }) => {
            requestUrl = url;
            requestBody = body;
            requestHeaders = config?.headers || {};
            return { data: { output: { choices: [{ message: { content: [{ image: "https://example.com/result.png" }] } }] } } };
        }) as typeof axios.post;
        const model = "qwen-image-3.0-pro";
        const channel = createModelChannel({
            id: "bailian-test",
            name: "百炼测试",
            baseUrl: "https://workspace.cn-beijing.maas.aliyuncs.com/compatible-mode/v1",
            apiKey: "synthetic-test-key",
            models: [model],
            modelCosts: [{ model, capability: "image", protocol: "bailian-image", billingMode: "fixed_request", unitPriceMicrocredits: 0 }],
        });
        const selectedModel = `${channel.id}::${model}`;

        await requestGeneration({ ...defaultConfig, channels: [channel], model: selectedModel, imageModel: selectedModel, size: "1024x1024", count: "1" }, "生成商品主图");

        expect(requestUrl).toBe("/api/ai/custom");
        expect(requestHeaders["x-canvas-upstream-url"]).toBe("https://workspace.cn-beijing.maas.aliyuncs.com/api/v1/services/aigc/multimodal-generation/generation");
        expect(requestBody).toMatchObject({
            model,
            input: { messages: [{ content: [{ text: "生成商品主图" }] }] },
            parameters: { n: 1, watermark: false, size: "1024*1024" },
        });
    });

    test("统一解析 Responses、Chat Completions 和 Gemini 工具调用", () => {
        expect(
            parseToolResponse({
                output: [{ type: "function_call", call_id: "call-1", name: "search", arguments: '{"q":"canvas"}' }],
            }),
        ).toMatchObject({ toolCalls: [{ id: "call-1", function: { name: "search", arguments: '{"q":"canvas"}' } }] });
        expect(
            parseChatCompletionPayload({
                choices: [{ message: { content: "done", tool_calls: [{ id: "call-2", function: { name: "search", arguments: "{}" } }] } }],
            }),
        ).toMatchObject({ content: "done", toolCalls: [{ id: "call-2", function: { name: "search" } }] });
        expect(
            parseGeminiToolResponse({
                candidates: [
                    {
                        content: {
                            parts: [
                                { text: "thinking", thought: true },
                                { functionCall: { id: "call-3", name: "search", args: { q: "canvas" } }, thoughtSignature: "sig" },
                            ],
                        },
                    },
                ],
            }),
        ).toMatchObject({ reasoning: "thinking", toolCalls: [{ id: "call-3", thoughtSignature: "sig", function: { name: "search", arguments: '{"q":"canvas"}' } }] });
    });

    test("SSE 增量解析保留正文、reasoning 和工具调用", () => {
        const responseState = { buffer: "", text: "", reasoning: "" };
        consumeResponseStreamText(responseState, 'data: {"type":"response.output_text.delta","delta":"hel"}\n\n');
        consumeResponseStreamText(responseState, 'data: {"type":"response.reasoning.delta","delta":"why"}\n\n');
        consumeResponseStreamText(responseState, 'data: {"type":"response.output_text.delta","delta":"lo"}', undefined, undefined, true);
        expect(responseState).toMatchObject({ text: "hello", reasoning: "why", buffer: "" });

        const chatState = { buffer: "", text: "", reasoning: "", toolCalls: new Map() };
        consumeChatCompletionStreamText(chatState, 'data: {"choices":[{"delta":{"content":"ok","reasoning_content":"r"}}]}\n\n');
        expect(chatState).toMatchObject({ text: "ok", reasoning: "r" });

        const geminiState = { buffer: "", text: "", reasoning: "", toolCalls: [] };
        consumeGeminiStreamText(geminiState, 'data: {"candidates":[{"content":{"parts":[{"text":"hi"}]}}]}\n\n');
        expect(geminiState).toMatchObject({ text: "hi" });
    });
});
