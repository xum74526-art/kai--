import { storyWorlds } from "./auth-landing-data";
import { useAuthMediaSwitcher } from "./use-auth-media-switcher";

export function StoryWorldsSection() {
    const media = useAuthMediaSwitcher(storyWorlds);
    const requestedWorld = storyWorlds[media.loadRequest.index];
    const activeWorld = storyWorlds[media.committedIndex];
    const status = media.phase === "loading"
        ? `正在加载${requestedWorld.label}`
        : media.phase === "error"
            ? `${requestedWorld.label}加载失败`
            : media.playing
                ? `${activeWorld.label}正在播放`
                : `${activeWorld.label}已暂停`;
    const action = media.phase === "error" ? "重试" : media.playing ? "暂停" : "播放";

    return (
        <section className="story-section page-shell" id="worlds" aria-labelledby="worlds-title">
            <div className="section-heading reveal">
                <h2 id="worlds-title">故事改变，<br />画面规则也要跟着变</h2>
                <p>不是给同一段剧情换皮，而是让人物、空间、光线和节奏一起改变。</p>
            </div>

            <div className="story-workspace">
                <div
                    className={`story-stage${media.phase === "loading" ? " is-changing" : ""}`}
                    ref={media.stageRef}
                    aria-busy={media.phase === "loading"}
                >
                    <figure>
                        <video
                            key={media.loadRequest.token}
                            ref={media.videoRef}
                            src={media.shouldLoad ? requestedWorld.clip : undefined}
                            muted
                            playsInline
                            preload="metadata"
                            poster={requestedWorld.poster}
                            aria-label={`${requestedWorld.title}动态画面`}
                            onLoadedData={(event) => media.handleReady(event, media.loadRequest.token, media.loadRequest.index)}
                            onCanPlay={(event) => media.handleReady(event, media.loadRequest.token, media.loadRequest.index)}
                            onError={() => media.handleError(media.loadRequest.token, media.loadRequest.index)}
                            onTimeUpdate={media.handleTimeUpdate}
                            onEnded={media.handleEnded}
                            onPlay={media.handlePlay}
                            onPause={media.handlePause}
                        />
                        <div className="showreel-topline">
                            <span><i aria-hidden="true" />{status}</span>
                            <button
                                className="media-control"
                                type="button"
                                onClick={media.phase === "error" ? media.retry : media.togglePlayback}
                                disabled={media.phase === "loading"}
                                aria-label={media.phase === "error" ? `重新加载${requestedWorld.title}` : `${action}${activeWorld.title}`}
                            >
                                {media.phase === "loading" ? "加载中" : action}
                            </button>
                        </div>
                        {media.phase === "ready" ? (
                            <figcaption>
                                <p>{activeWorld.eyebrow}</p>
                                <h3>{activeWorld.title}</h3>
                                <p>{activeWorld.copy}</p>
                            </figcaption>
                        ) : null}
                        {media.phase === "error" ? (
                            <figcaption role="alert">
                                <p>视频加载失败</p>
                                <h3>{requestedWorld.title}</h3>
                                <p>本地片段暂时无法读取，请重试。</p>
                            </figcaption>
                        ) : null}
                    </figure>
                </div>

                <div className="story-choices reveal" role="group" aria-label="选择故事的画面规则">
                    {storyWorlds.map((world, index) => {
                        const active = media.committedIndex === index;
                        const pending = media.pendingIndex === index;
                        return (
                            <button
                                className={`${active ? "is-active" : ""}${pending ? " is-pending" : ""}`}
                                type="button"
                                key={world.title}
                                aria-pressed={active}
                                aria-busy={pending}
                                onClick={() => media.select(index)}
                            >
                                <span>{world.label}</span>
                                <strong>{world.title}</strong>
                                <p>{world.tags.join("、")}</p>
                            </button>
                        );
                    })}
                </div>
            </div>
        </section>
    );
}
