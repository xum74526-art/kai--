import { useEffect, useRef, useState, type FormEvent } from "react";
import { useReducedMotion } from "motion/react";

import { publicAssetPath } from "@/lib/app-base-path";
import { apiOptions, creationSources, type ApiKind, type GalleryCategory } from "./auth-landing-data";

const styleAsset = (name: string) => publicAssetPath(`short-drama-styles/${name}`);
const authAsset = (path: string) => publicAssetPath(`auth-showcase/${path}`);

const modeDetails: Record<ApiKind, { description: string }> = {
    text: { description: "把故事整理成可以继续编辑的梗概、场景和分镜。" },
    audio: { description: "为这一幕准备旁白、对白、音效与配乐方向。" },
    video: { description: "生成动态镜头，并继续编辑运动、景别和节奏。" },
};

const sourceClassName: Record<string, string> = {
    character: "source-character",
    scene: "source-scene",
    style: "source-style",
    tech: "source-tech",
};

type CreationLabSectionProps = {
    selectedApi: ApiKind;
    onApiChange: (api: ApiKind) => void;
    onRequestAuth: () => void;
};

export function CreationLabSection({ selectedApi, onApiChange, onRequestAuth }: CreationLabSectionProps) {
    const [selected, setSelected] = useState<Set<string>>(() => new Set(
        creationSources.filter((source) => source.selected).map((source) => source.id),
    ));
    const [prompt, setPrompt] = useState("清晨的雾林里，一位独自赶路的年轻女人停在岔路口。树影像水墨一样在薄雾中晕开，镜头从远景缓慢跟随，最后停在她回头确认来路的一瞬。");
    const [status, setStatus] = useState("");
    const [combining, setCombining] = useState(false);
    const promptRef = useRef<HTMLTextAreaElement>(null);
    const combineTimerRef = useRef<number | null>(null);
    const count = selected.size;
    const selectedOption = apiOptions.find((option) => option.value === selectedApi) ?? apiOptions[2];

    useEffect(() => () => {
        if (combineTimerRef.current !== null) window.clearTimeout(combineTimerRef.current);
    }, []);

    const toggleSource = (id: string) => {
        setSelected((current) => {
            const next = new Set(current);
            if (next.has(id)) next.delete(id);
            else next.add(id);
            return next;
        });
        setStatus("");
    };

    const combine = () => {
        if (count === 0) {
            setStatus("请先选择至少一项素材。");
            return;
        }
        if (combineTimerRef.current !== null) window.clearTimeout(combineTimerRef.current);
        setCombining(false);
        window.requestAnimationFrame(() => setCombining(true));
        combineTimerRef.current = window.setTimeout(() => setCombining(false), 900);
        setStatus(`已将 ${count} 项素材组合为创作上下文。`);
        promptRef.current?.focus();
    };

    const submit = (event: FormEvent<HTMLFormElement>) => {
        event.preventDefault();
        if (count === 0) {
            setStatus("请先选择至少一项素材。");
            return;
        }
        if (!prompt.trim()) {
            setStatus("先写下一句创作描述。");
            promptRef.current?.focus();
            return;
        }
        setStatus(`已准备 ${count} 项素材和${selectedOption.title}，请先登录进入正式画布。`);
        onRequestAuth();
    };

    return (
        <section className="creation-section" id="create" aria-labelledby="create-title">
            <div className="page-shell">
                <div className="section-heading creation-heading reveal">
                    <h2 id="create-title">选好素材，<br />直接写下一幕</h2>
                    <p>人物、场景、质感和技术在同一处形成创作上下文。</p>
                </div>

                <div className="creation-panel reveal">
                    <div className={`material-orbit${combining ? " is-combining" : ""}`} role="group" aria-label="素材汇聚画布">
                        {creationSources.map((source) => (
                            <div
                                className={`route route-${source.id}${selected.has(source.id) ? "" : " is-dormant"}`}
                                data-active={selected.has(source.id)}
                                key={`route-${source.id}`}
                                aria-hidden="true"
                            >
                                <span />
                            </div>
                        ))}

                        {creationSources.map((source) => {
                            const active = selected.has(source.id);
                            return (
                                <button
                                    className={`source-card ${sourceClassName[source.id]}${active ? " is-selected" : ""}`}
                                    type="button"
                                    aria-pressed={active}
                                    key={source.id}
                                    onClick={() => toggleSource(source.id)}
                                >
                                    <img src={source.image} alt="" />
                                    <span><small>{source.type}</small><strong>{source.title}</strong></span>
                                </button>
                            );
                        })}

                        <div className={`orbit-center${combining ? " is-complete" : ""}`}>
                            <button type="button" disabled={count === 0} aria-label="组合已选择的素材" onClick={combine}>+</button>
                            <strong>组合素材</strong>
                            <span>已选择 <b>{count}</b> 项</span>
                        </div>
                    </div>

                    <form className="prompt-panel" onSubmit={submit}>
                        <div>
                            <h3>写下这一幕</h3>
                            <p>{modeDetails[selectedApi].description}</p>
                        </div>
                        <div className="mode-switch" role="group" aria-label="选择创作 API">
                            {apiOptions.map((option) => {
                                const active = selectedApi === option.value;
                                return (
                                    <button
                                        className={active ? "is-active" : undefined}
                                        type="button"
                                        aria-pressed={active}
                                        key={option.value}
                                        onClick={() => onApiChange(option.value)}
                                    >
                                        {option.label}
                                    </button>
                                );
                            })}
                        </div>
                        <label className="prompt-field">
                            <span>创作描述</span>
                            <textarea
                                ref={promptRef}
                                name="prompt"
                                rows={6}
                                value={prompt}
                                onChange={(event) => {
                                    setPrompt(event.target.value);
                                    setStatus("");
                                }}
                                placeholder="写下人物动作、场景变化与镜头节奏。"
                            />
                        </label>
                        <div className="prompt-meta" role="group" aria-label="生成参数">
                            <span>16:9</span><span>6 秒</span><span>{selectedOption.title}</span>
                        </div>
                        <button className="button button-light prompt-submit" type="submit">带入画布</button>
                        <p className="prompt-note">登录后可在正式画布中继续这段创作。</p>
                        <p className="prompt-status" role="status" aria-live="polite">{status}</p>
                    </form>
                </div>
            </div>
        </section>
    );
}

const galleryFilters: Array<{ value: "all" | GalleryCategory; label: string }> = [
    { value: "all", label: "全部" },
    { value: "live", label: "真人" },
    { value: "animation", label: "动画" },
    { value: "world", label: "世界观" },
];

const showcaseItems: Array<{ category: GalleryCategory; className: string; image: string; alt: string; title: string; type: string }> = [
        { category: "live", className: "gallery-a", image: styleAsset("real-life.jpg"), alt: "城市十字路口", title: "十字路口", type: "城市实景" },
        { category: "animation", className: "gallery-b", image: styleAsset("comic-pop.jpg"), alt: "粉蓝复古电脑与掌机", title: "像素年代", type: "复古电子" },
        { category: "world", className: "gallery-c", image: styleAsset("cyberpunk-neon.jpg"), alt: "霓虹夜市街区", title: "霓虹街市", type: "类型影像" },
        { category: "live", className: "gallery-d", image: styleAsset("period-live-action.jpg"), alt: "海湾群岛与船只", title: "群岛晨航", type: "自然实景" },
        { category: "world", className: "gallery-e", image: styleAsset("space-opera.jpg"), alt: "太空舱俯瞰地球", title: "近地轨道", type: "太空歌剧" },
        { category: "animation", className: "gallery-f", image: styleAsset("chinese-2d.jpg"), alt: "红色背景的东方动画角色", title: "赤红角色", type: "二维动画" },
        { category: "animation", className: "gallery-g", image: styleAsset("clay-stop-motion.jpg"), alt: "手工泥塑人物", title: "土塑人物", type: "定格动画" },
        { category: "live", className: "gallery-h", image: styleAsset("retro-hong-kong.jpg"), alt: "复古香港电车街景", title: "港岛午后", type: "年代质感" },
        { category: "animation", className: "gallery-i", image: authAsset("chapters/frame-105.png"), alt: "雪夜和果子店少女", title: "雪夜小店", type: "治愈动画" },
        { category: "world", className: "gallery-j", image: styleAsset("storybook-fantasy.jpg"), alt: "红色岩壁间的公路", title: "荒原公路", type: "公路影像" },
];

export function ShowcaseSection() {
    const [filter, setFilter] = useState<"all" | GalleryCategory>("all");
    return (
        <section className="gallery-section page-shell" id="gallery" aria-labelledby="gallery-title">
            <div className="section-heading gallery-heading reveal">
                <h2 id="gallery-title">从现实出发，<br />也可以走进陌生世界</h2>
                <p>从实拍、动画到世界观设定，让每一种故事找到合适的画面语言。</p>
            </div>
            <div className="gallery-filter reveal" role="group" aria-label="筛选视觉素材">
                {galleryFilters.map((item) => {
                    const active = filter === item.value;
                    return <button className={active ? "is-active" : undefined} type="button" aria-pressed={active} key={item.value} onClick={() => setFilter(item.value)}>{item.label}</button>;
                })}
            </div>
            <div className="gallery-grid">
                {showcaseItems.map((item) => {
                    const hidden = filter !== "all" && item.category !== filter;
                    return (
                        <figure className={`gallery-card ${item.className} reveal${hidden ? " is-hidden" : ""}`} hidden={hidden} key={`${item.category}-${item.title}`}>
                            <img src={item.image} alt={item.alt} loading="lazy" />
                            <figcaption><strong>{item.title}</strong><span>{item.type}</span></figcaption>
                        </figure>
                    );
                })}
            </div>
        </section>
    );
}

export function ClosingSection({ onRequestAuth }: { onRequestAuth: () => void }) {
    const visualRef = useRef<HTMLDivElement>(null);
    const videoRef = useRef<HTMLVideoElement>(null);
    const manualPauseRef = useRef(false);
    const reducedMotion = Boolean(useReducedMotion());
    const [visible, setVisible] = useState(false);
    const [playing, setPlaying] = useState(false);
    const [failed, setFailed] = useState(false);

    useEffect(() => {
        const visual = visualRef.current;
        if (!visual || !("IntersectionObserver" in window)) {
            setVisible(true);
            return;
        }
        const scrollRoot = visual.closest<HTMLElement>(".auth-landing");
        const observer = new IntersectionObserver(([entry]) => setVisible(entry.isIntersecting), {
            root: scrollRoot,
            rootMargin: "160px 0px",
            threshold: 0.05,
        });
        observer.observe(visual);
        return () => observer.disconnect();
    }, []);

    useEffect(() => {
        const handleVisibility = () => {
            const video = videoRef.current;
            if (!video) return;
            if (document.hidden) video.pause();
            else if (visible && !reducedMotion && !manualPauseRef.current) void video.play().catch(() => setPlaying(false));
        };
        document.addEventListener("visibilitychange", handleVisibility);
        return () => document.removeEventListener("visibilitychange", handleVisibility);
    }, [reducedMotion, visible]);

    useEffect(() => {
        const video = videoRef.current;
        if (!video) return;
        if (!visible || reducedMotion) video.pause();
        else if (!manualPauseRef.current) void video.play().catch(() => setPlaying(false));
    }, [reducedMotion, visible]);

    const togglePlayback = () => {
        const video = videoRef.current;
        if (!video) return;
        if (video.paused) {
            manualPauseRef.current = false;
            void video.play().catch(() => setPlaying(false));
        } else {
            manualPauseRef.current = true;
            video.pause();
        }
    };

    const retry = () => {
        const video = videoRef.current;
        if (!video) return;
        setFailed(false);
        video.load();
        if (visible && !reducedMotion && !manualPauseRef.current) void video.play().catch(() => setPlaying(false));
    };

    return (
        <section className="closing-section page-shell" aria-labelledby="closing-title">
            <div className="closing-panel reveal" ref={visualRef}>
                <video
                    ref={videoRef}
                    src={authAsset("videos/closing-60.mp4")}
                    muted
                    playsInline
                    loop
                    preload="metadata"
                    poster={authAsset("chapters/frame-60.png")}
                    aria-label="虚拟舞台动态影像"
                    onPlay={() => setPlaying(true)}
                    onPause={() => setPlaying(false)}
                    onError={() => setFailed(true)}
                    onCanPlay={() => {
                        const video = videoRef.current;
                        if (video && visible && !reducedMotion && !manualPauseRef.current) void video.play().catch(() => setPlaying(false));
                    }}
                />
                <div className="closing-shade" aria-hidden="true" />
                <div className="closing-copy">
                    <h2 id="closing-title">从一帧开始，<br />继续写你的下一幕</h2>
                    <p>把人物、场景、声音和运动带进同一个画布，登录后继续完成镜头。</p>
                    <div><button className="button button-light" type="button" onClick={onRequestAuth}>进入 KAI影</button><a href="#top">重新看动态作品</a></div>
                </div>
                <button className="closing-pause" type="button" aria-label={`${playing ? "暂停" : "播放"}虚拟舞台视频`} onClick={togglePlayback}>{playing ? "暂停" : "播放"}</button>
                <div className="media-error" role="alert" hidden={!failed}><strong>收尾视频加载失败</strong><button type="button" onClick={retry}>重新加载</button></div>
            </div>
        </section>
    );
}
