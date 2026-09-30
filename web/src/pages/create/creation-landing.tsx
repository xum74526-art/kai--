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

const SHOWCASE_ASSET_REVISION = "20260910-desktop-seedance";
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
        label: "霓虹雨城",
        headline: "跟着霓虹灯线，穿过雨夜街区",
        subtitle: "雨幕、广告牌与湿地反光，把城市节奏带进镜头。",
        title: "霓虹雨城",
        meta: "雨夜街道，低位前推",
        prompt: "镜头沿雨夜未来都市的湿润街道低位前推，青色与粉色霓虹灯牌映在路面，高楼在薄雾中逐层展开。",
        mode: "video",
        video: showcaseAsset("director-neon-rain-city.mp4"),
        poster: showcaseAsset("director-neon-rain-city.png"),
        thumbnail: showcaseAsset("director-neon-rain-city.png"),
        description: "用雨幕、霓虹反射和纵深街道建立未来城市的夜行节奏。",
        goal: "让观众进入潮湿而明亮的未来街区",
        lens: "低机位前推，广角纵深",
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
        label: "金色原野",
        headline: "让光线掠过原野，把时间写进风景",
        subtitle: "云层、树影与起伏地形，共同组织画面的呼吸。",
        title: "金色原野",
        meta: "暖阳草坡，缓慢横移",
        prompt: "金色夕阳照亮连绵起伏的草坡，一棵枝叶繁茂的大树立在原野中央，远处红色农舍升起炊烟，镜头缓慢横移，云影与风吹草浪轻轻掠过画面。",
        mode: "video",
        video: showcaseAsset("director-golden-countryside.mp4"),
        poster: showcaseAsset("director-golden-countryside.png"),
        thumbnail: showcaseAsset("director-golden-countryside.png"),
        description: "以暖阳、草坡、独树和红色农舍组织宁静开阔的田园远景。",
        goal: "让风景呈现温暖而舒展的时间感",
        lens: "全景横移，平稳远观",
        status: "可用",
    },
    {
        id: "commerce",
        label: "水墨山河",
        headline: "用留白与墨色，建立东方景深",
        subtitle: "山影、孤舟与水面倒影，在克制运动中展开。",
        title: "水墨山河",
        meta: "远山孤舟，贴水慢移",
        prompt: "水墨群山笼罩在薄雾中，一叶孤舟从古松与山石之间缓慢驶过，镜头贴近平静水面横移，金色微光在墨色倒影中闪动。",
        mode: "video",
        video: showcaseAsset("director-ink-mountain-lake.mp4"),
        poster: showcaseAsset("director-ink-mountain-lake.png"),
        thumbnail: showcaseAsset("director-ink-mountain-lake.png"),
        description: "通过水墨层次、雾气留白和孤舟运动建立东方山水意境。",
        goal: "让传统画意呈现安静而连贯的镜头感",
        lens: "贴水横移，远景留白",
        status: "可用",
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
            <section className="creation-stage" aria-label="KAI影创作舞台">
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
                    <p className="creation-stage-kicker"><Video aria-hidden="true" />KAI影导演台</p>
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
