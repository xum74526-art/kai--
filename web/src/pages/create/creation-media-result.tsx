import { useEffect, useMemo, useRef, useState, type CSSProperties } from "react";
import { App, Image, Modal } from "antd";
import { Maximize2 } from "lucide-react";

import { creationMediaAspectRatio } from "./creation-assets";

type CreationMediaType = "image" | "video";
type FullscreenVideoElement = HTMLVideoElement & {
    webkitEnterFullscreen?: () => void;
    webkitRequestFullscreen?: () => Promise<void> | void;
};

type CreationMediaFrameStyle = CSSProperties & {
    "--creation-media-aspect": string;
    "--creation-media-frame-width": string;
};

const IMAGE_RESULT_MAX_WIDTH = 320;
const IMAGE_RESULT_MAX_HEIGHT = 568;
const SINGLE_IMAGE_RESULT_MAX_WIDTH = 680;
const SINGLE_IMAGE_RESULT_MAX_HEIGHT = 680;
const VIDEO_RESULT_MAX_WIDTH = 680;
const VIDEO_RESULT_MAX_HEIGHT = 540;

function aspectRatioValue(value: string | undefined, type: CreationMediaType) {
    const [width, height] = creationMediaAspectRatio(value, type).split("/").map((part) => Number(part.trim()));
    return Number.isFinite(width) && Number.isFinite(height) && width > 0 && height > 0 ? width / height : type === "video" ? 16 / 9 : 1;
}

function mediaFrameStyle(aspect: number, type: CreationMediaType, single: boolean): CreationMediaFrameStyle {
    // 首次按请求画幅稳定占位，媒体加载后按真实宽高校正；限制视觉尺寸但不再用固定框制造留边。
    const maxWidth = type === "video" ? VIDEO_RESULT_MAX_WIDTH : single ? SINGLE_IMAGE_RESULT_MAX_WIDTH : IMAGE_RESULT_MAX_WIDTH;
    const maxHeight = type === "video" ? VIDEO_RESULT_MAX_HEIGHT : single ? SINGLE_IMAGE_RESULT_MAX_HEIGHT : IMAGE_RESULT_MAX_HEIGHT;
    const frameWidth = Math.min(maxWidth, maxHeight * aspect);
    return {
        "--creation-media-aspect": String(aspect),
        "--creation-media-frame-width": `${Math.round(frameWidth)}px`,
    };
}

function CreationResultFrame({ url, type, ratio, single, onPreview }: {
    url: string;
    type: CreationMediaType;
    ratio?: string;
    single: boolean;
    onPreview: () => void;
}) {
    const fallbackAspect = useMemo(() => aspectRatioValue(ratio, type), [ratio, type]);
    const [aspect, setAspect] = useState(fallbackAspect);

    useEffect(() => setAspect(fallbackAspect), [fallbackAspect, url]);

    const updateAspect = (width: number, height: number) => {
        if (!Number.isFinite(width) || !Number.isFinite(height) || width <= 0 || height <= 0) return;
        const next = width / height;
        setAspect((current) => Math.abs(current - next) < 0.001 ? current : next);
    };

    if (type === "video") {
        return <button type="button" className="creation-video-result" style={mediaFrameStyle(aspect, type, true)} onClick={onPreview} aria-label="放大预览生成视频">
            <video muted playsInline preload="metadata" src={url} onLoadedMetadata={(event) => updateAspect(event.currentTarget.videoWidth, event.currentTarget.videoHeight)} />
            <span><Maximize2 />预览视频</span>
        </button>;
    }

    return <button type="button" className="creation-image-result" style={mediaFrameStyle(aspect, type, single)} onClick={onPreview} aria-label="放大预览生成图片">
        <img src={url} alt="生成结果" loading="lazy" onLoad={(event) => updateAspect(event.currentTarget.naturalWidth, event.currentTarget.naturalHeight)} />
        <span aria-hidden="true"><Maximize2 /></span>
    </button>;
}

export function CreationResultMedia({ urls, type, ratio }: { urls: string[]; type: CreationMediaType; ratio?: string }) {
    const [previewUrl, setPreviewUrl] = useState("");
    const single = urls.length === 1;

    return <>
        {type === "video"
            ? <CreationResultFrame url={urls[0]} type="video" ratio={ratio} single onPreview={() => setPreviewUrl(urls[0])} />
            : <div className={`creation-image-result-grid${single ? " is-single" : ""}`}>{urls.map((url) => <CreationResultFrame key={url} url={url} type="image" ratio={ratio} single={single} onPreview={() => setPreviewUrl(url)} />)}</div>}
        <CreationMediaPreviewModal url={previewUrl} urls={type === "image" ? urls : undefined} type={type} onClose={() => setPreviewUrl("")} />
    </>;
}

function CreationImagePreview({ url, urls, onClose }: { url: string; urls?: string[]; onClose: () => void }) {
    const items = useMemo(() => (urls?.length ? urls : url ? [url] : []).map((src, index) => ({ src, alt: `生成结果 ${index + 1}` })), [url, urls]);
    const requestedIndex = Math.max(0, items.findIndex((item) => item.src === url));
    const [current, setCurrent] = useState(requestedIndex);

    return <Image.PreviewGroup
        items={items}
        classNames={{ popup: { root: "creation-image-preview" } }}
        preview={{
            open: Boolean(url),
            current,
            minScale: 1,
            maxScale: 8,
            scaleStep: 0.5,
            movable: true,
            countRender: (active, total) => `${active} / ${total}`,
            onChange: (next) => setCurrent(next),
            onOpenChange: (open) => { if (!open) onClose(); },
        }}
    />;
}

function CreationVideoPreview({ url, onClose }: { url: string; onClose: () => void }) {
    const videoRef = useRef<HTMLVideoElement>(null);
    const { message } = App.useApp();

    const openFullscreen = async () => {
        const video = videoRef.current as FullscreenVideoElement | null;
        if (!video) return;
        const requestFullscreen = video.requestFullscreen
            ? () => video.requestFullscreen()
            : video.webkitEnterFullscreen
                ? () => video.webkitEnterFullscreen!()
                : video.webkitRequestFullscreen
                    ? () => video.webkitRequestFullscreen!()
                    : undefined;
        if (!requestFullscreen) {
            message.info("当前浏览器请使用播放器右下角的全屏按钮");
            return;
        }
        try {
            await requestFullscreen();
        } catch (error) {
            message.error(error instanceof Error ? `无法进入全屏：${error.message}` : "无法进入全屏，请使用播放器右下角的全屏按钮");
        }
    };

    return <Modal open={Boolean(url)} title={null} footer={null} centered destroyOnHidden width="min(1200px, calc(100vw - 32px))" onCancel={onClose} className="creation-media-preview-modal" styles={{ body: { padding: 0 } }}>
        {url ? <div className="creation-video-preview-stage">
            <video ref={videoRef} controls autoPlay playsInline className="creation-media-preview-video" src={url} />
            <button type="button" className="creation-video-preview-fullscreen" onClick={openFullscreen}><Maximize2 />全屏播放</button>
        </div> : null}
    </Modal>;
}

export function CreationMediaPreviewModal({ url, urls, type, onClose }: { url: string; urls?: string[]; type: CreationMediaType; onClose: () => void }) {
    return type === "image" ? <CreationImagePreview key={url || "closed"} url={url} urls={urls} onClose={onClose} /> : <CreationVideoPreview url={url} onClose={onClose} />;
}
