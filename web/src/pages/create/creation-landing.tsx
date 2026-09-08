import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import {
    FileText,
    FolderOpen,
    History,
    Pause,
    Play,
    Video,
    X,
} from "lucide-react";
import { useReducedMotion } from "motion/react";

import { publicAssetPath } from "@/lib/app-base-path";
import "./creation-landing.css";

const SHOWCASE_ASSET_REVISION = "20260908-clarity";
// 固定文件名的展示素材替换后，用版本参数避免浏览器继续显示旧缓存。
const showcaseAsset = (name: string) => publicAssetPath(`create-showcase/${name}?v=${SHOWCASE_ASSET_REVISION}`);
const styleAsset = (name: string) => publicAssetPath(`short-drama-styles/${name}`);

type CreationLandingMode = "text" | "image" | "video";
type CreationSceneId = "story" | "space" | "motion" | "world" | "commerce";

type CreationScene = {
    id: CreationSceneId;
    label: string;
    headline: string;
    subtitle: string;
    title: string;
    meta: string;
    prompt: string;
    mode: CreationLandingMode;
    video: string | null;
    poster: string;
    thumbnail: string;
    description: string;
    goal: string;
    lens: string;
    status: string;
    preview?: boolean;
};

const creationScenes: CreationScene[] = [
    {
        id: "story",
        label: "雨窗故事",
        headline: "把一个想法，推成下一场戏",
        subtitle: "故事、镜头与素材，在同一导演台里继续。",
        title: "雨窗城市",
        meta: "玻璃倒影，缓慢推进",
        prompt: "雨夜的车窗里，一座陌生城市正在倒退。镜头缓慢贴近玻璃上的倒影。",
        mode: "video",
        video: showcaseAsset("director-rain-window-smooth.mp4"),
        poster: showcaseAsset("director-rain-window-smooth.jpg"),
        thumbnail: styleAsset("ink-narrative.jpg"),
        description: "在雨夜车窗与城市倒影之间建立缓慢推进的镜头。",
        goal: "让观众先感到陌生与等待",
        lens: "近景慢推，玻璃反射",
        status: "可用",
    },
    {
        id: "space",
        label: "折叠空间",
        headline: "先让空间成立，再安排镜头穿行",
        subtitle: "用建筑、视线与光线，搭出可延展的场景。",
        title: "折叠空间",
        meta: "天井光线，向内旋转",
        prompt: "镜头穿过一座向内旋转的建筑，天井光线逐层落下，空间像记忆一样缓慢折叠。",
        mode: "video",
        video: showcaseAsset("director-spiral-architecture-safe.mp4"),
        poster: showcaseAsset("director-spiral-architecture-safe.jpg"),
        thumbnail: styleAsset("retro-hong-kong.jpg"),
        description: "从建筑关系和光线入口出发，确定镜头的穿行路径。",
        goal: "先建立清楚的空间方向",
        lens: "广角穿行，向上抬升",
        status: "可用",
    },
    {
        id: "motion",
        label: "海洋运动",
        headline: "把运动路径和镜头速度分开写",
        subtitle: "主体动作与机位节奏清楚，动态才容易控制。",
        title: "海洋运动",
        meta: "贴近海床，向前跟随",
        prompt: "一只蝠鲼掠过珊瑚礁上方，镜头贴近海床向前跟随，阳光穿过水面形成移动的光束。",
        mode: "video",
        video: showcaseAsset("director-underwater-coral-safe.mp4"),
        poster: showcaseAsset("director-underwater-coral-safe.jpg"),
        thumbnail: styleAsset("real-life.jpg"),
        description: "用主体运动、机位速度和水下光线控制镜头节奏。",
        goal: "让运动保持平稳与轻盈",
        lens: "低机位跟随，慢速推进",
        status: "可用",
    },
    {
        id: "world",
        label: "轨道世界",
        headline: "先统一规则，再让角色进入世界",
        subtitle: "材质、尺度与色彩保持一致，镜头才能连续。",
        title: "轨道世界",
        meta: "行星光环，低空航行",
        prompt: "一艘小型飞行器沿行星光环低空航行，远处城市依附在轨道结构上，整体保持冷灰材质。",
        mode: "video",
        video: showcaseAsset("director-cosmic-orbit.mp4"),
        poster: showcaseAsset("director-cosmic-orbit.jpg"),
        thumbnail: styleAsset("clay-stop-motion.jpg"),
        description: "先确定尺度、材质与色彩规则，再组织世界中的镜头。",
        goal: "建立可复用的世界规则",
        lens: "远景航行，缓慢侧移",
        status: "可用",
    },
    {
        id: "commerce",
        label: "商品视觉",
        headline: "把商品信息整理成视觉方案",
        subtitle: "主体、卖点与渠道规格，继续沿用现有创作流程。",
        title: "商品视觉",
        meta: "银灰材质，红色轮廓光",
        prompt: "一款深灰色便携音箱置于磨砂金属台面，侧后方红色轮廓光勾勒结构，镜头缓慢环绕。",
        mode: "image",
        video: null,
        poster: showcaseAsset("commerce-speaker.png"),
        thumbnail: showcaseAsset("commerce-speaker.png"),
        description: "在现有创作流程中增加商品主体、卖点和渠道规格入口。",
        goal: "突出结构、材质与核心卖点",
        lens: "中近景环绕，细节特写",
        status: "方案预览",
        preview: true,
    },
];

export function CreationLanding({ composer, onUsePrompt, onOpenLibrary, onOpenHistory }: {
    composer: ReactNode;
    onUsePrompt: (mode: CreationLandingMode, prompt: string) => void;
    onOpenLibrary: () => void;
    onOpenHistory: () => void;
}) {
    const reducedMotion = Boolean(useReducedMotion());
    const videoRef = useRef<HTMLVideoElement>(null);
    const initialPromptAppliedRef = useRef(false);
    const [activeSceneId, setActiveSceneId] = useState<CreationSceneId>("story");
    const [videoReady, setVideoReady] = useState(false);
    const [videoFailed, setVideoFailed] = useState(false);
    const [videoPaused, setVideoPaused] = useState(false);
    const [contextOpen, setContextOpen] = useState(false);

    const activeScene = useMemo(
        () => creationScenes.find((scene) => scene.id === activeSceneId) ?? creationScenes[0],
        [activeSceneId],
    );
    const videoEnabled = Boolean(activeScene.video) && !reducedMotion && !videoFailed;

    useEffect(() => {
        if (initialPromptAppliedRef.current) return;
        initialPromptAppliedRef.current = true;
        onUsePrompt(activeScene.mode, activeScene.prompt);
    }, [activeScene.mode, activeScene.prompt, onUsePrompt]);

    useEffect(() => {
        setVideoReady(false);
        setVideoFailed(false);
        setVideoPaused(false);
    }, [activeSceneId]);

    useEffect(() => {
        const video = videoRef.current;
        if (!video || !videoEnabled || !videoReady || videoPaused) {
            video?.pause();
            return;
        }

        const syncPlayback = () => {
            if (document.hidden) {
                video.pause();
                return;
            }
            void video.play().catch(() => setVideoPaused(true));
        };

        syncPlayback();
        document.addEventListener("visibilitychange", syncPlayback);
        return () => {
            document.removeEventListener("visibilitychange", syncPlayback);
            video.pause();
        };
    }, [activeSceneId, videoEnabled, videoPaused, videoReady]);

    const selectScene = (scene: CreationScene) => {
        setActiveSceneId(scene.id);
        setContextOpen(false);
        onUsePrompt(scene.mode, scene.prompt);
    };

    const togglePlayback = () => {
        if (!videoEnabled) return;
        setVideoPaused((paused) => !paused);
    };

    return (
        <div className="creation-landing" data-scene={activeScene.id}>
            <section className="creation-stage" aria-label="北斗 KAI影创作舞台">
                <img
                    key={`${activeScene.id}-poster`}
                    className="creation-stage-media creation-stage-poster"
                    src={activeScene.poster}
                    alt=""
                    aria-hidden="true"
                    decoding="async"
                />
                {videoEnabled ? (
                    <video
                        key={activeScene.video}
                        ref={videoRef}
                        className={`creation-stage-media creation-stage-video${videoReady ? " is-ready" : ""}`}
                        src={activeScene.video ?? undefined}
                        poster={activeScene.poster}
                        muted
                        loop
                        playsInline
                        preload="metadata"
                        onLoadedData={() => setVideoReady(true)}
                        onError={() => {
                            setVideoFailed(true);
                            setVideoPaused(false);
                        }}
                    />
                ) : null}
                <span className="creation-stage-wash" aria-hidden="true" />
                <span className="creation-stage-grain" aria-hidden="true" />

                <div className="creation-stage-controls">
                    <button type="button" className="creation-stage-control" onClick={() => setContextOpen(true)}>
                        <FileText aria-hidden="true" />
                        <span>创作上下文</span>
                    </button>
                    <button type="button" className="creation-stage-control is-icon" onClick={onOpenHistory} aria-label="打开创作历史" title="创作历史">
                        <History aria-hidden="true" />
                    </button>
                    {activeScene.video ? (
                        <button type="button" className="creation-stage-control" onClick={togglePlayback} disabled={!videoEnabled} aria-label={videoPaused ? "播放背景视频" : "暂停背景视频"}>
                            {videoPaused || !videoEnabled ? <Play aria-hidden="true" /> : <Pause aria-hidden="true" />}
                            <span>{videoPaused || !videoEnabled ? "播放" : "暂停"}</span>
                        </button>
                    ) : null}
                </div>

                <header className="creation-stage-hero" aria-live="polite">
                    <p className="creation-stage-kicker"><Video aria-hidden="true" />北斗导演台</p>
                    <h1>{activeScene.headline}</h1>
                    <p>{activeScene.subtitle}</p>
                </header>

                <aside className="creation-stage-caption" aria-label="当前创作场景">
                    <span className={activeScene.preview ? "is-preview" : undefined}>{activeScene.status}</span>
                    <h2>{activeScene.title}</h2>
                    <p>{activeScene.meta}</p>
                    <button type="button" onClick={() => setContextOpen(true)}>查看镜头信息</button>
                </aside>

                <div className="creation-stage-zone">
                    <div className="creation-stage-scenes" role="tablist" aria-label="切换创作场景">
                        <span className="creation-stage-scenes-label">切换场景</span>
                        {creationScenes.map((scene) => {
                            const active = scene.id === activeScene.id;
                            return (
                                <button
                                    key={scene.id}
                                    type="button"
                                    role="tab"
                                    aria-selected={active}
                                    className={active ? "is-active" : undefined}
                                    onClick={() => selectScene(scene)}
                                >
                                    <img src={scene.thumbnail} alt="" aria-hidden="true" />
                                    <span>{scene.label}</span>
                                </button>
                            );
                        })}
                    </div>

                    <section className="creation-stage-console" aria-label="创作输入">
                        <div className="creation-stage-console-heading">
                            <span>创作描述</span>
                            <strong>{activeScene.title}</strong>
                        </div>
                        <div className="creation-stage-composer">{composer}</div>
                    </section>
                </div>

                <button
                    type="button"
                    className={`creation-stage-drawer-backdrop${contextOpen ? " is-open" : ""}`}
                    aria-label="关闭创作上下文"
                    tabIndex={contextOpen ? 0 : -1}
                    onClick={() => setContextOpen(false)}
                />
                <aside className={`creation-stage-drawer${contextOpen ? " is-open" : ""}`} aria-hidden={!contextOpen} aria-label="创作上下文详情">
                    <div className="creation-stage-drawer-heading">
                        <div><span>当前场景</span><h2>{activeScene.title}</h2></div>
                        <button type="button" onClick={() => setContextOpen(false)} aria-label="关闭创作上下文"><X aria-hidden="true" /></button>
                    </div>
                    <p>{activeScene.description}</p>
                    <dl>
                        <div><dt>故事目标</dt><dd>{activeScene.goal}</dd></div>
                        <div className="is-current"><dt>镜头与参考</dt><dd>{activeScene.lens}</dd></div>
                        <div><dt>生成与选择</dt><dd>确认模型后进入生成</dd></div>
                        <div><dt>带入画布</dt><dd>继续编排镜头与版本</dd></div>
                    </dl>
                    <button type="button" className="creation-stage-library-button" onClick={() => {
                        setContextOpen(false);
                        onOpenLibrary();
                    }}>
                        <FolderOpen aria-hidden="true" />
                        从素材库开始
                    </button>
                </aside>
            </section>
        </div>
    );
}
