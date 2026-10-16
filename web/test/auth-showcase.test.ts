import { describe, expect, test } from "bun:test";

import { AUTH_SHOWCASE_CLIPS, resolveAuthShowcaseClips } from "../src/pages/auth/auth-showcase-data";
import { normalizePublicAppearance } from "../src/stores/use-appearance-store";

describe("login showcase configuration", () => {
    test("unconfigured media shows three new films before four returning films", () => {
        const appearance = normalizePublicAppearance({});

        expect(resolveAuthShowcaseClips(appearance)).toBe(AUTH_SHOWCASE_CLIPS);
        expect(AUTH_SHOWCASE_CLIPS.map(({ id, title, video, poster }) => [id, title, video, poster])).toEqual([
            ["neon-rain-city", "霓虹雨城", "/create-showcase/director-neon-rain-city.mp4", "/create-showcase/director-neon-rain-city.png"],
            ["golden-countryside", "金色原野", "/create-showcase/director-golden-countryside.mp4", "/create-showcase/director-golden-countryside.png"],
            ["ink-mountain-lake", "水墨山河", "/create-showcase/director-ink-mountain-lake.mp4", "/create-showcase/director-ink-mountain-lake.png"],
            ["bamboo-chase", "竹林追影", "/auth-showcase/videos/world-30.mp4", "/auth-showcase/chapters/frame-30.png"],
            ["neon-duel", "霓虹对决", "/auth-showcase/videos/world-45.mp4", "/auth-showcase/chapters/frame-45.png"],
            ["country-summer", "乡野夏日", "/auth-showcase/videos/world-90.mp4", "/auth-showcase/chapters/frame-90.png"],
            ["virtual-stage", "虚拟舞台", "/auth-showcase/videos/closing-60.mp4", "/auth-showcase/chapters/frame-60.png"],
        ]);
        expect(new Set(AUTH_SHOWCASE_CLIPS.map(({ id }) => id)).size).toBe(7);
        expect(new Set(AUTH_SHOWCASE_CLIPS.map(({ video }) => video)).size).toBe(7);
    });

    test("a configured poster alone preserves the saved video and poster", () => {
        const appearance = normalizePublicAppearance({
            brandName: "Test Studio",
            authVideoPosterConfigured: true,
            authVideoPosterUrl: "/api/public/appearance/assets/poster?v=poster-only",
        });

        expect(resolveAuthShowcaseClips(appearance)).toEqual([{
            id: "configured-brand-video",
            title: "Test Studio品牌影片",
            video: appearance.authVideoUrl,
            poster: "/api/public/appearance/assets/poster?v=poster-only",
        }]);
    });

    test("a configured video without a poster never adopts a built-in poster", () => {
        const appearance = normalizePublicAppearance({
            authVideoConfigured: true,
            authVideoUrl: "/api/public/appearance/assets/video?v=video-only",
            authVideoPosterUrl: "",
        });

        expect(resolveAuthShowcaseClips(appearance)).toEqual([{
            id: "configured-brand-video",
            title: `${appearance.brandName}品牌影片`,
            video: "/api/public/appearance/assets/video?v=video-only",
            poster: "",
        }]);
    });

    test("a configured video and poster replace the entire default playlist", () => {
        const appearance = normalizePublicAppearance({
            authVideoConfigured: true,
            authVideoPosterConfigured: true,
            authVideoUrl: "/api/public/appearance/assets/video?v=custom-pair",
            authVideoPosterUrl: "/api/public/appearance/assets/poster?v=custom-pair",
        });

        expect(resolveAuthShowcaseClips(appearance)).toEqual([{
            id: "configured-brand-video",
            title: `${appearance.brandName}品牌影片`,
            video: appearance.authVideoUrl,
            poster: appearance.authVideoPosterUrl,
        }]);
    });

    test("resolving either playlist does not change autoplay or saved appearance", () => {
        for (const configured of [false, true]) {
            for (const autoplay of [false, true]) {
                const appearance = Object.freeze(normalizePublicAppearance({
                    brandName: "Saved Studio",
                    brandSlug: "saved-studio",
                    authHeroTitle: "已保存的标题",
                    authHeroDescription: "已保存的说明",
                    logoConfigured: true,
                    logoUrl: "/api/public/appearance/assets/logo?v=saved",
                    logoFrameEnabled: false,
                    authVideoConfigured: configured,
                    authVideoPosterConfigured: configured,
                    authVideoUrl: "/api/public/appearance/assets/video?v=saved",
                    authVideoPosterUrl: "/api/public/appearance/assets/poster?v=saved",
                    authVideoAutoplay: autoplay,
                    revision: "saved-revision",
                }));
                const before = structuredClone(appearance);

                const clips = resolveAuthShowcaseClips(appearance);

                expect(clips).toHaveLength(configured ? 1 : 7);
                expect(appearance.authVideoAutoplay).toBe(autoplay);
                expect(appearance).toEqual(before);
            }
        }
    });

    test("all selected film and poster references resolve to nonempty bundled files", async () => {
        for (const clip of AUTH_SHOWCASE_CLIPS) {
            for (const path of [clip.video, clip.poster]) {
                const file = Bun.file(new URL(`../public${path}`, import.meta.url));
                expect(await file.exists()).toBe(true);
                expect(file.size).toBeGreaterThan(0);
            }
        }
    });
});

// 这些源码断言只约束接线与清理边界，不能替代真实浏览器的媒体事件验收。
describe("login showcase playback source contracts", () => {
    test("the resolved configuration feeds only one active video", async () => {
        const source = await Bun.file(new URL("../src/pages/auth/auth-showcase.tsx", import.meta.url)).text();

        expect(source).toContain("useAppearanceStore((state) => state.resolved)");
        expect(source).toContain("if (!resolved)");
        expect(source.indexOf("if (!resolved)")).toBeLessThan(source.indexOf("return <AuthShowcasePlayer"));
        expect(source).toContain("useAuthShowcasePlayer(clips, autoplay)");
        expect(source).toContain("JSON.stringify([clips, appearance.authVideoAutoplay])");
        expect(source).toContain("{media.shouldLoad ? (");
        expect(source.match(/<video\b/g)).toHaveLength(1);
        expect(source).toContain("src={clip.video}");
        expect(source).toContain("poster={clip.poster || undefined}");
        expect(source).toContain("onError={media.handleError}");
        expect(source).toContain("onEnded={media.handleEnded}");
        expect(source).toContain("onClick={failed ? retry : media.togglePlayback}");
        expect(source).toContain("aria-pressed={index === media.index}");
        expect(source).toContain("media.select(index)");
        expect(source).not.toContain("autoPlay");
    });

    test("the player guards stale events and pauses outside the visible page", async () => {
        const source = await Bun.file(new URL("../src/pages/auth/use-auth-showcase-player.ts", import.meta.url)).text();

        expect(source).toContain("autoplay && reducedMotion === false");
        expect(source).toContain('intentRef.current === "play"');
        expect(source).toContain('intentRef.current === "auto" && autoAllowedRef.current');
        expect(source).toContain("event.currentTarget === videoRef.current && event.currentTarget === activeVideoRef.current");
        expect(source).toContain("revisionRef.current === revision");
        expect(source).toContain("operationRef.current === operation");
        expect(source).toContain("!document.hidden && wantsPlayback()");
        expect(source).toContain('document.addEventListener("visibilitychange", handleVisibility)');
        expect(source).toContain('document.removeEventListener("visibilitychange", handleVisibility)');
        expect(source).toContain("new IntersectionObserver");
        expect(source).toContain("observer?.disconnect()");
        expect(source).toContain("videoRef.current?.pause()");
        expect(source).toContain("if (!isCurrentEvent(event)) return;");
    });
});
