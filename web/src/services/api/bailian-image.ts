import { nanoid } from "nanoid";

import { isSystemProxyBaseUrl, resolveBackendApiUrl } from "@/stores/use-config-store";

const BAILIAN_IMAGE_PATH = "/api/v1/services/aigc/image-generation/generation";

export type BailianImagePayload = {
    model: string;
    input: {
        messages: Array<{
            role: "user";
            content: Array<{ image: string } | { text: string }>;
        }>;
    };
    parameters: {
        n: number;
        watermark: false;
        size?: string;
    };
};

export type BailianImageResponse = {
    output?: {
        choices?: Array<{
            message?: {
                content?: Array<{ image?: string }>;
            };
        }>;
    };
    code?: string;
    message?: string;
    request_id?: string;
};

/** 百炼兼容模式与原生异步图片接口同源，但路径不共享。 */
export function bailianImageApiUrl(baseUrl: string) {
    const normalizedBaseUrl = resolveBackendApiUrl(baseUrl).trim().replace(/\/+$/, "");
    if (isSystemProxyBaseUrl(normalizedBaseUrl)) return `${normalizedBaseUrl}${BAILIAN_IMAGE_PATH}`;

    const url = new URL(normalizedBaseUrl);
    const path = url.pathname.replace(/\/+$/, "");
    if (path.toLowerCase().endsWith(BAILIAN_IMAGE_PATH)) return url.toString().replace(/\/+$/, "");
    const compatibleModeIndex = path.toLowerCase().indexOf("/compatible-mode/");
    const apiV1Index = path.toLowerCase().indexOf("/api/v1");
    const rootPath = compatibleModeIndex >= 0 ? path.slice(0, compatibleModeIndex) : apiV1Index >= 0 ? path.slice(0, apiV1Index) : path.replace(/\/v1$/i, "");
    url.pathname = `${rootPath}${BAILIAN_IMAGE_PATH}`.replace(/\/{2,}/g, "/");
    url.search = "";
    url.hash = "";
    return url.toString();
}

export function buildBailianImagePayload(input: { model: string; prompt: string; images: string[]; count: number; size?: string }): BailianImagePayload {
    const model = input.model.trim();
    const normalizedModel = model.toLowerCase();
    const isWan27 = normalizedModel.startsWith("wan2.7-image");
    const size = normalizeBailianImageSize(input.size, isWan27);
    if (isWan27 && input.images.length > 0 && size === "4K") throw new Error("万相 2.7 参考图编辑最高支持 2K，请降低图片尺寸后重试");

    return {
        model,
        input: {
            messages: [
                {
                    role: "user",
                    content: [...input.images.map((image) => ({ image })), { text: input.prompt }],
                },
            ],
        },
        parameters: {
            n: input.count,
            watermark: false,
            ...(size ? { size } : {}),
        },
    };
}

export function parseBailianImagePayload(payload: BailianImageResponse) {
    if (payload.code) throw new Error(payload.message || payload.code);
    const images =
        payload.output?.choices
            ?.flatMap((choice) => choice.message?.content || [])
            .map((item) => item.image)
            .filter((value): value is string => Boolean(value))
            .map((dataUrl) => ({ id: nanoid(), dataUrl })) || [];
    if (!images.length) throw new Error("百炼接口没有返回图片");
    return images;
}

function normalizeBailianImageSize(size: string | undefined, isWan27: boolean) {
    const normalized = (size || "").trim();
    if (!normalized || normalized.toLowerCase() === "auto") return undefined;
    if (isWan27 && /^[124]k$/i.test(normalized)) return normalized.toUpperCase();
    return normalized.replace(/^(\d+)x(\d+)$/i, "$1*$2");
}
