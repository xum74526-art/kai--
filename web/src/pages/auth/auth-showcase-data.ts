import type { PublicAppearance } from "@/services/api/appearance";

export type AuthShowcaseClip = {
    id: string;
    title: string;
    video: string;
    poster: string;
};

// 三段新素材优先展示，再接回四段旧片；全部复用原文件，不转码或截取。
export const AUTH_SHOWCASE_CLIPS: readonly AuthShowcaseClip[] = [
    {
        id: "neon-rain-city",
        title: "霓虹雨城",
        video: "/create-showcase/director-neon-rain-city.mp4",
        poster: "/create-showcase/director-neon-rain-city.png",
    },
    {
        id: "golden-countryside",
        title: "金色原野",
        video: "/create-showcase/director-golden-countryside.mp4",
        poster: "/create-showcase/director-golden-countryside.png",
    },
    {
        id: "ink-mountain-lake",
        title: "水墨山河",
        video: "/create-showcase/director-ink-mountain-lake.mp4",
        poster: "/create-showcase/director-ink-mountain-lake.png",
    },
    {
        id: "bamboo-chase",
        title: "竹林追影",
        video: "/auth-showcase/videos/world-30.mp4",
        poster: "/auth-showcase/chapters/frame-30.png",
    },
    {
        id: "neon-duel",
        title: "霓虹对决",
        video: "/auth-showcase/videos/world-45.mp4",
        poster: "/auth-showcase/chapters/frame-45.png",
    },
    {
        id: "country-summer",
        title: "乡野夏日",
        video: "/auth-showcase/videos/world-90.mp4",
        poster: "/auth-showcase/chapters/frame-90.png",
    },
    {
        id: "virtual-stage",
        title: "虚拟舞台",
        video: "/auth-showcase/videos/closing-60.mp4",
        poster: "/auth-showcase/chapters/frame-60.png",
    },
];

export function resolveAuthShowcaseClips(appearance: PublicAppearance): readonly AuthShowcaseClip[] {
    // 视频和封面可独立配置；任一自定义时都不能被内置轮播覆盖。
    if (appearance.authVideoConfigured || appearance.authVideoPosterConfigured) {
        return [{
            id: "configured-brand-video",
            title: `${appearance.brandName}品牌影片`,
            video: appearance.authVideoUrl,
            poster: appearance.authVideoPosterUrl,
        }];
    }
    return AUTH_SHOWCASE_CLIPS;
}
