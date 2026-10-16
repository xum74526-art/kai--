import { useMemo, useState } from "react";
import { ChevronLeft, ChevronRight, Film, Pause, Play, RotateCcw } from "lucide-react";

import { useAppearanceStore } from "@/stores/use-appearance-store";
import { resolveAuthShowcaseClips, type AuthShowcaseClip } from "./auth-showcase-data";
import { useAuthShowcasePlayer } from "./use-auth-showcase-player";

export function AuthShowcase() {
    const appearance = useAppearanceStore((state) => state.appearance);
    const resolved = useAppearanceStore((state) => state.resolved);
    const clips = useMemo(() => resolveAuthShowcaseClips(appearance), [appearance]);

    // 公开外观尚未读取完成时不请求默认视频，避免先闪现错误的品牌内容。
    if (!resolved) {
        return <div className="auth-showcase-frame auth-showcase-placeholder" role="status">正在读取展示配置</div>;
    }

    return <AuthShowcasePlayer key={JSON.stringify([clips, appearance.authVideoAutoplay])} clips={clips} autoplay={appearance.authVideoAutoplay} />;
}

function AuthShowcasePlayer({ clips, autoplay }: { clips: readonly AuthShowcaseClip[]; autoplay: boolean }) {
    const media = useAuthShowcasePlayer(clips, autoplay);
    const clip = clips[media.index];
    const [failedPoster, setFailedPoster] = useState<string | null>(null);
    const failed = media.phase === "error";
    const loading = media.phase === "loading";
    const playbackRequested = media.playing || media.playbackRequested;
    const action = failed ? "重试" : playbackRequested ? "暂停" : "播放";
    const ActionIcon = failed ? RotateCcw : playbackRequested ? Pause : Play;
    const status = failed ? "视频暂时无法播放，请重试。"
        : media.phase === "blocked" ? "自动播放未开启，点击播放即可观看。"
        : loading ? "正在加载视频" : media.playing ? "正在播放" : "已暂停";
    const hasPoster = Boolean(clip.poster && failedPoster !== clip.poster);
    const retry = () => {
        setFailedPoster(null);
        media.retry();
    };

    return (
        <div className="auth-showcase">
            <div ref={media.stageRef} className="auth-showcase-frame" aria-busy={loading}>
                {hasPoster && media.phase !== "ready" ? (
                    <img key={clip.poster} className="auth-showcase-media" src={clip.poster} alt="" decoding="async" onError={() => setFailedPoster(clip.poster)} />
                ) : null}
                {!hasPoster && media.phase !== "ready" ? (
                    <div className="auth-showcase-placeholder">
                        <Film className="size-8" aria-hidden="true" />
                        <span>{failed ? "影片加载失败" : loading ? "正在加载影片" : "点击播放观看影片"}</span>
                    </div>
                ) : null}
                {media.shouldLoad ? (
                    <video
                        key={media.revision}
                        ref={media.videoRef}
                        className="auth-showcase-media auth-showcase-video"
                        data-ready={media.phase === "ready"}
                        src={clip.video}
                        poster={clip.poster || undefined}
                        aria-label={`${clip.title}视频`}
                        muted
                        playsInline
                        preload="metadata"
                        onLoadedData={media.handleLoaded}
                        onCanPlay={media.handleLoaded}
                        onPlay={media.handlePlay}
                        onPause={media.handlePause}
                        onError={media.handleError}
                        onEnded={media.handleEnded}
                    >
                        你的浏览器暂不支持视频播放。
                    </video>
                ) : null}
            </div>
            <div className="auth-showcase-toolbar">
                <div className="min-w-0">
                    <p className="truncate text-sm font-medium" title={clip.title}>{clip.title}</p>
                    <p className="auth-scene-muted mt-1 text-xs" role={failed ? "alert" : "status"}>{status}</p>
                </div>
                <div className="flex shrink-0 items-center gap-1">
                    {clips.length > 1 ? <button type="button" className="auth-showcase-control" aria-label="上一部作品" onClick={() => media.select((media.index + clips.length - 1) % clips.length)}><ChevronLeft className="size-4" aria-hidden="true" /></button> : null}
                    <button type="button" className="auth-showcase-control" aria-label={`${action}视频`} aria-pressed={!failed && playbackRequested} onClick={failed ? retry : media.togglePlayback}>
                        <ActionIcon className="size-4" aria-hidden="true" />
                        <span>{action}</span>
                    </button>
                    {clips.length > 1 ? <button type="button" className="auth-showcase-control" aria-label="下一部作品" onClick={() => media.select((media.index + 1) % clips.length)}><ChevronRight className="size-4" aria-hidden="true" /></button> : null}
                </div>
            </div>
            {clips.length > 1 ? (
                <div className="auth-showcase-clips" role="group" aria-label="选择展示作品">
                    {clips.map((item, index) => (
                        <button key={item.id} type="button" className="auth-showcase-clip" aria-label={`观看${item.title}`} aria-pressed={index === media.index} onClick={() => media.select(index)}>
                            <img src={item.poster} alt="" loading="lazy" decoding="async" />
                            <span>{item.title}</span>
                        </button>
                    ))}
                </div>
            ) : null}
        </div>
    );
}
