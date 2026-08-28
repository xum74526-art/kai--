import { modelCapabilityConfigFor, videoResolutionRequest } from "@/lib/model-capabilities";
import { boolConfig, normalizeSeedanceRatio, normalizeSeedanceResolution } from "@/lib/seedance-video";
import { getResourceOSSUrl } from "@/services/api/resources";
import { modelOptionName } from "@/stores/use-config-store";
import type { ReferenceAudio, ReferenceVideo } from "@/types/media";
import type { ReferenceImage } from "@/types/image";

import type { ApiEnvelope, ApiVideoResponse, RequestOptions, ResolvedAiConfig, VideoGenerationTask, VideoGenerationTaskState } from "./video-contracts";
import type { VideoProviderDeps } from "./video-provider-deps";
import { normalizeVideoSeconds, normalizeVideoSize } from "./video-validation";

export async function createVideoGenerationsTask(deps: VideoProviderDeps, config: ResolvedAiConfig, model: string, prompt: string, references: ReferenceImage[], videoReferences: ReferenceVideo[], audioReferences: ReferenceAudio[], options?: RequestOptions): Promise<VideoGenerationTask> {
    if (references.length > 9 || videoReferences.length > 3 || audioReferences.length > 3) throw new Error("NewAPI Video Generations 最多支持 9 张参考图、3 个参考视频和 3 个参考音频");
    const isAiApiWay = config.interfaceType === "aiapiway-video";
    if (isAiApiWay && audioReferences.length > 0 && references.length === 0 && videoReferences.length === 0) throw new Error("AI API WAY 的参考音频不能单独使用，请同时提供参考图片或参考视频");
    if (!isAiApiWay && audioReferences.length > 0 && videoReferences.length === 0) throw new Error("NewAPI Video Generations 的参考音频必须同时提供至少 1 个参考视频；纯音频生视频请切换到支持该模式的渠道");
    const [imageUrls, videoUrls, audioUrls] = await Promise.all([
        Promise.all(references.map((item) => resolveVideoGenerationsUrl(item.url || item.dataUrl, item.storageKey))),
        Promise.all(videoReferences.map((item) => resolveVideoGenerationsUrl(item.url, item.storageKey))),
        Promise.all(audioReferences.map((item) => resolveVideoGenerationsUrl(item.url, item.storageKey))),
    ]);
    const profile = modelCapabilityConfigFor(config, model).video!;
    const modelName = modelOptionName(model);
    const resolution = newAPIVideoResolutionRequest(profile, config.vquality, modelName);
    const mediaPayload = {
        ...(imageUrls.length ? { image_urls: imageUrls } : {}),
        ...(videoUrls.length ? { video_urls: videoUrls } : {}),
        ...(audioUrls.length ? { audio_urls: audioUrls } : {}),
    };
    const payload = isAiApiWay ? buildAiApiWayPayload(config, profile, modelName, prompt, mediaPayload) : {
        model: modelName,
        prompt: prompt.trim(),
        seconds: normalizeVideoSeconds(config.videoSeconds),
        aspect_ratio: normalizeVideoSize(config.size) || "16:9",
        ...(resolution ? { resolution } : {}),
        ...(profile.generateAudio.supported ? { generate_audio: boolConfig(config.videoGenerateAudio, profile.generateAudio.default) } : {}),
        ...mediaPayload,
    };
    try {
        const created = deps.response.unwrapVideoResponse(await deps.transport.post<ApiVideoResponse>(deps.transport.apiUrl("/video/generations"), payload, options));
        const id = deps.response.videoTaskId(created);
        if (!id) throw new Error("NewAPI Video Generations 没有返回任务 ID");
        return { id, provider: "video-generations", model };
    } catch (error) {
        throw new Error(deps.response.readAxiosError(error, "NewAPI Video Generations 任务创建失败"));
    }
}

function buildAiApiWayPayload(
    config: ResolvedAiConfig,
    profile: NonNullable<ReturnType<typeof modelCapabilityConfigFor>["video"]>,
    model: string,
    prompt: string,
    media: { image_urls?: string[]; video_urls?: string[]; audio_urls?: string[] },
) {
    const normalizedRatio = normalizeSeedanceRatio(config.size);
    const resolution = normalizeAiApiWayResolution(config.vquality, model);
    const content = [
        ...(media.image_urls || []).map((url) => ({ type: "image_url", role: "reference_image", image_url: { url } })),
        ...(media.video_urls || []).map((url) => ({ type: "video_url", role: "reference_video", video_url: { url } })),
        ...(media.audio_urls || []).map((url) => ({ type: "audio_url", role: "reference_audio", audio_url: { url } })),
    ];
    return {
        model,
        prompt: prompt.trim(),
        metadata: {
            resolution,
            duration: Number(normalizeVideoSeconds(config.videoSeconds)),
            ratio: normalizedRatio === "adaptive" ? profile.defaultRatio || "16:9" : normalizedRatio,
            generate_audio: boolConfig(config.videoGenerateAudio, profile.generateAudio.default),
            ...(profile.watermark.supported ? { watermark: boolConfig(config.videoWatermark, profile.watermark.default) } : {}),
            ...(content.length ? { content } : {}),
        },
    };
}

function normalizeAiApiWayResolution(value: string, model: string) {
    const resolution = normalizeSeedanceResolution(value, model);
    if (resolution !== "2160p") return resolution;
    if (model.toLowerCase().includes("filter-off")) return "4k";
    throw new Error("AI API WAY 的 4K 输出只支持 filter-off 海外版模型");
}

export async function pollVideoGenerationsTask(deps: VideoProviderDeps, task: VideoGenerationTask, options?: RequestOptions): Promise<VideoGenerationTaskState> {
    try {
        const raw = await deps.transport.get<ApiEnvelope<Record<string, unknown>>>(deps.transport.apiUrl(`/video/generations/${encodeURIComponent(task.id)}`), options);
        const state = deps.response.unwrapEnvelopeRecord(raw);
        const status = String(state.status || "").toUpperCase();
        if (status === "SUCCESS" || status === "SUCCEEDED" || status === "COMPLETED") {
            const url = String(state.result_url || state.video_url || state.url || "");
            if (!url) return { status: "failed", error: "视频任务已完成但没有返回视频地址" };
            return { status: "completed", result: await deps.response.videoResultFromUrl(url, options) };
        }
        if (status === "FAILURE" || status === "FAILED" || status === "CANCELLED") return { status: "failed", error: String(state.fail_reason || state.error || "视频生成失败") };
        return { status: "pending" };
    } catch (error) {
        throw new Error(deps.response.readAxiosError(error, "NewAPI Video Generations 任务查询失败"));
    }
}

function newAPIVideoResolutionRequest(profile: NonNullable<ReturnType<typeof modelCapabilityConfigFor>["video"]>, value: string, model: string) {
    if (model.trim().toLowerCase() === "grok-video-1.5-1080p") return "1080p";
    return videoResolutionRequest(profile, value);
}

async function resolveVideoGenerationsUrl(value: string | undefined, storageKey?: string) {
    if (storageKey?.startsWith("resource:")) return getResourceOSSUrl(storageKey);
    if (isPublicMediaUrl(value || "")) return String(value);
    throw new Error("NewAPI Video Generations 的参考素材需要公网 URL；请先把素材保存到对象存储");
}

function isPublicMediaUrl(value: string) {
    return /^https?:\/\//i.test(value || "");
}
