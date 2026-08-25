import { publicAssetPath } from "@/lib/app-base-path";

const chapterAsset = (name: string) => publicAssetPath(`auth-showcase/chapters/${name}`);
const styleAsset = (name: string) => publicAssetPath(`short-drama-styles/${name}`);
const videoAsset = (name: string) => publicAssetPath(`auth-showcase/videos/${name}`);

export type ApiKind = "text" | "audio" | "video";

export const apiOptions: Array<{ value: ApiKind; label: string; title: string; description: string }> = [
    { value: "text", label: "文本", title: "文本 API", description: "剧本理解、改写与分镜规划" },
    { value: "audio", label: "音频", title: "音频 API", description: "语音合成、识别与配乐生成" },
    { value: "video", label: "视频", title: "视频 API", description: "文生视频、图生视频与镜头编辑" },
];

export const reelChapters = [
    { kicker: "真人武侠", title: "竹林追影", copy: "动作先建立方向，人物与环境才会进入同一个镜头。", poster: chapterAsset("frame-30.png"), clip: videoAsset("world-30.mp4"), playTo: 7.55 },
    { kicker: "城市动画", title: "霓虹对决", copy: "霓虹、反射与快速运动，让城市也成为冲突的一部分。", poster: chapterAsset("frame-45.png"), clip: videoAsset("world-45.mp4"), playTo: 5.85 },
    { kicker: "治愈动画", title: "乡野夏日", copy: "风、树影与生活动作，为没有台词的片刻留下呼吸。", poster: chapterAsset("frame-90.png"), clip: videoAsset("world-90.mp4"), playTo: 4.2 },
    { kicker: "三维动画", title: "虚拟舞台", copy: "角色、音乐与空间装置一起进入镜头，让舞台成为故事发生的世界。", poster: chapterAsset("frame-60.png"), clip: videoAsset("closing-60.mp4"), playTo: 5.8 },
] as const;

export const storyWorlds = [
    { label: "高空实拍", eyebrow: "极限纪实", title: "让地形决定速度", copy: "贴崖飞行把距离和速度同时压进镜头，云海与岩壁共同放大风险感。", detail: "用超广角交代地形，再让飞行轨迹贴着岩壁掠过，速度便有了清晰参照。", tags: ["贴崖跟拍", "超广角", "高速运动"], poster: chapterAsset("frame-75.png"), clip: videoAsset("story-wingsuit.mp4"), playTo: 3.3, alt: "翼装飞行者掠过云海岩壁" },
    { label: "雪夜动画", eyebrow: "治愈动画", title: "用暖光留住停顿", copy: "雪夜街道与店内暖光形成温差，一个递出和果子的动作就能把情绪安静地落下来。", detail: "让冷色环境托住暖色人物，再用微小动作和停顿拉近观众与角色的距离。", tags: ["冷暖对比", "微动作", "雪夜氛围"], poster: chapterAsset("frame-105.png"), clip: videoAsset("story-snow-shop.mp4"), playTo: 7.4, alt: "雪夜和果子店里的动画少女" },
    { label: "黑白悬疑", eyebrow: "黑白电影", title: "让反差先说话", copy: "雨水、烟雾和侧光切出人物轮廓，停顿比对白更早暴露他的犹疑。", detail: "先让高反差光线建立立场，再以雨声和近景停顿把悬念留在人物脸上。", tags: ["高反差", "雨夜特写", "悬疑节奏"], poster: chapterAsset("frame-120.png"), clip: videoAsset("story-noir.mp4"), playTo: 7.45, alt: "雨夜中点烟的黑白电影人物" },
    { label: "校园动画", eyebrow: "青春动画", title: "把关系写进走位", copy: "并肩、回望和被风吹动的花瓣，让人物关系在一次放学路上自然发生变化。", detail: "用双人距离和视线变化代替解释，让春日色彩与柔和横移承接情绪。", tags: ["双人调度", "春日色彩", "柔和横移"], poster: chapterAsset("frame-165.png"), clip: videoAsset("story-campus.mp4"), playTo: 4.6, alt: "樱花校园天台上的动画学生" },
] as const;

export const creationSources = [
    { id: "character", className: "source-card-character", image: styleAsset("urban-live-action.jpg"), type: "人物", title: "都市旅人", selected: true },
    { id: "scene", className: "source-card-scene", image: styleAsset("nature-healing.jpg"), type: "场景", title: "林间晨光", selected: true },
    { id: "style", className: "source-card-style", image: styleAsset("ink-narrative.jpg"), type: "风格", title: "水墨晕染", selected: true },
    { id: "tech", className: "source-card-tech", image: styleAsset("future-tech.jpg"), type: "技术", title: "3D 打印实验室", selected: false },
] as const;

export type GalleryCategory = "live" | "animation" | "world";
