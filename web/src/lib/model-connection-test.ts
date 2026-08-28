import { requestAudioGeneration } from "@/services/api/audio";
import { runBackendGenerationTask } from "@/services/api/generation-task";
import { requestImageQuestion } from "@/services/api/image";
import { createVideoGenerationTask } from "@/services/api/video";
import { defaultConfig, encodeChannelModel, type ModelCapability, type ModelChannel } from "@/stores/use-config-store";
import type { ModelProtocol } from "@/lib/model-protocols";

type ModelConnectionTestOptions = {
    onTaskCreated?: (taskId: string) => void;
};

export async function testChannelModelConnection(channel: ModelChannel, model: string, capability: ModelCapability, protocol: ModelProtocol, options?: ModelConnectionTestOptions) {
    if (!channel.baseUrl.trim()) throw new Error("请先填写 Base URL");
    if (!channel.apiKey.trim()) throw new Error("请先填写 API Key");
    const selectedModel = encodeChannelModel(channel.id, model);
    const modelCost = channel.modelCosts?.find((item) => item.model === model);
    const testProtocol = channel.apiFormat === "gemini" && !modelCost?.protocol ? undefined : protocol;
    const testChannel: ModelChannel = {
        ...channel,
        models: channel.models.includes(model) ? channel.models : [...channel.models, model],
        modelCosts: [
            {
                model,
                displayName: modelCost?.displayName,
                capability,
                protocol: testProtocol,
                billingMode: modelCost?.billingMode || "fixed_request",
                unitPriceMicrocredits: modelCost?.unitPriceMicrocredits || 0,
                capabilityConfig: modelCost?.capabilityConfig,
            },
            ...(channel.modelCosts || []).filter((item) => item.model !== model),
        ],
    };
    const config = {
        ...defaultConfig,
        channelMode: "local" as const,
        baseUrl: channel.baseUrl,
        apiKey: channel.apiKey,
        apiFormat: channel.apiFormat,
        channels: [testChannel],
        model: selectedModel,
        imageModel: selectedModel,
        videoModel: selectedModel,
        textModel: selectedModel,
        audioModel: selectedModel,
        models: [selectedModel],
        imageModels: capability === "image" ? [selectedModel] : [],
        videoModels: capability === "video" ? [selectedModel] : [],
        textModels: capability === "text" ? [selectedModel] : [],
        audioModels: capability === "audio" ? [selectedModel] : [],
        count: "1",
        size: capability === "image" ? "1024x1024" : "16:9",
        videoSeconds: "6",
        vquality: "720",
        videoGenerateAudio: "false",
    };

    switch (capability) {
        case "text":
            await requestImageQuestion(config, [{ role: "user", content: "Reply with OK." }], () => undefined);
            return "文本响应正常";
        case "image": {
            let taskId = "";
            // 图片测试必须经过任务中心。供应商即使生成数分钟，浏览器也只发短轮询请求，
            // 不再把整次生成绑定在反向代理的一条长连接上。
            const result = await runBackendGenerationTask({
                mode: "image",
                prompt: "A simple gray circle on a white background.",
                config,
                metadata: { source: "model-connection-test" },
                onTaskUpdate: (task) => {
                    if (taskId) return;
                    taskId = task.id;
                    options?.onTaskCreated?.(task.id);
                },
            });
            if (!result.images?.length) throw new Error("图片测试任务已完成，但没有返回可用图片");
            return taskId ? `图片生成正常（任务 ${taskId}）` : "图片生成正常";
        }
        case "audio":
            await requestAudioGeneration(config, "Model test.");
            return "音频生成正常";
        case "video": {
            const task = await createVideoGenerationTask(config, "A static gray circle on a white background.");
            return `视频任务已创建（${task.id}）`;
        }
    }
}
