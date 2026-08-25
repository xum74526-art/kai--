import { reelChapters } from "./auth-landing-data";
import { useAuthMediaSwitcher } from "./use-auth-media-switcher";

export function AuthShowreel() {
    const media = useAuthMediaSwitcher(reelChapters, { autoAdvance: true });
    const requestedChapter = reelChapters[media.loadRequest.index];
    const caption = reelChapters[media.committedIndex];
    const status = media.phase === "loading"
        ? "正在加载动态作品"
        : media.phase === "error"
            ? "动态作品加载失败"
            : media.playing
                ? "作品正在播放"
                : "作品已暂停";
    const action = media.phase === "error" ? "重试" : media.playing ? "暂停" : "播放";

    return (
        <div className="hero-media reveal" data-reveal-delay="1">
            <div
                className={`showreel-frame${media.phase === "loading" ? " is-changing" : ""}`}
                ref={media.stageRef}
                aria-busy={media.phase === "loading"}
            >
                <video
                    key={media.loadRequest.token}
                    ref={media.videoRef}
                    className="showreel-video"
                    src={media.shouldLoad ? requestedChapter.clip : undefined}
                    muted
                    playsInline
                    preload="metadata"
                    poster={requestedChapter.poster}
                    aria-label={`${requestedChapter.title}动态作品`}
                    onLoadedData={(event) => media.handleReady(event, media.loadRequest.token, media.loadRequest.index)}
                    onCanPlay={(event) => media.handleReady(event, media.loadRequest.token, media.loadRequest.index)}
                    onError={() => media.handleError(media.loadRequest.token, media.loadRequest.index)}
                    onTimeUpdate={media.handleTimeUpdate}
                    onEnded={media.handleEnded}
                    onPlay={media.handlePlay}
                    onPause={media.handlePause}
                >
                    你的浏览器暂不支持视频播放。
                </video>
                <div className="showreel-shade" aria-hidden="true" />
                <div className="showreel-topline">
                    <span><i aria-hidden="true" />{status}</span>
                    <button
                        className="media-control"
                        type="button"
                        onClick={media.phase === "error" ? media.retry : media.togglePlayback}
                        disabled={media.phase === "loading"}
                        aria-label={media.phase === "error" ? `重新加载${requestedChapter.title}` : `${action}动态作品`}
                    >
                        {media.phase === "loading" ? "加载中" : action}
                    </button>
                </div>
                {media.phase === "ready" ? (
                    <div className="showreel-caption">
                        <p>{caption.kicker}</p>
                        <h2>{caption.title}</h2>
                        <p>{caption.copy}</p>
                    </div>
                ) : null}
                {media.phase === "error" ? (
                    <div className="showreel-caption" role="alert">
                        <p>视频加载失败</p>
                        <h2>{requestedChapter.title}</h2>
                        <p>本地片段暂时无法读取，请重试。</p>
                    </div>
                ) : null}
            </div>

            <div className="reel-chapters" aria-label="选择动态作品">
                {reelChapters.map((chapter, index) => {
                    const active = media.committedIndex === index;
                    const pending = media.pendingIndex === index;
                    return (
                        <button
                            className={`chapter${active ? " is-active" : ""}${pending ? " is-pending" : ""}`}
                            type="button"
                            key={chapter.title}
                            aria-pressed={active}
                            aria-busy={pending}
                            onClick={() => media.select(index)}
                        >
                            <img src={chapter.poster} alt="" />
                            <span><small>{String(index + 1).padStart(2, "0")}</small>{chapter.title}</span>
                        </button>
                    );
                })}
            </div>
        </div>
    );
}
