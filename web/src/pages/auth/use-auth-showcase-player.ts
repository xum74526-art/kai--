import { useReducedMotion } from "motion/react";
import { useCallback, useEffect, useLayoutEffect, useRef, useState, type SyntheticEvent } from "react";

import type { AuthShowcaseClip } from "./auth-showcase-data";

type PlaybackPhase = "idle" | "loading" | "ready" | "blocked" | "error";
type PlaybackIntent = "auto" | "play" | "pause";
type VideoEvent = SyntheticEvent<HTMLVideoElement>;

export function useAuthShowcasePlayer(clips: readonly AuthShowcaseClip[], autoplay: boolean) {
    const reducedMotion = useReducedMotion();
    const stageRef = useRef<HTMLDivElement>(null);
    const videoRef = useRef<HTMLVideoElement>(null);
    const activeVideoRef = useRef<HTMLVideoElement | null>(null);
    const mountedRef = useRef(false);
    const visibleRef = useRef(false);
    const autoAllowedRef = useRef(autoplay && reducedMotion === false);
    const intentRef = useRef<PlaybackIntent>("auto");
    const phaseRef = useRef<PlaybackPhase>("idle");
    const loadRef = useRef(false);
    const indexRef = useRef(0);
    const revisionRef = useRef(0);
    const operationRef = useRef(0);
    const pendingPlayRef = useRef<number | null>(null);
    const [selection, setSelection] = useState({ index: 0, revision: 0 });
    const [shouldLoad, setShouldLoad] = useState(false);
    const [phase, setPhase] = useState<PlaybackPhase>("idle");
    const [playing, setPlaying] = useState(false);
    const [playbackRequested, setPlaybackRequested] = useState(false);

    const updatePhase = useCallback((next: PlaybackPhase) => {
        phaseRef.current = next;
        setPhase(next);
    }, []);
    const wantsPlayback = useCallback(() => intentRef.current === "play"
        || (intentRef.current === "auto" && autoAllowedRef.current), []);
    const canPlay = useCallback(() => mountedRef.current && visibleRef.current
        && !document.hidden && wantsPlayback(), [wantsPlayback]);
    const invalidatePlay = useCallback(() => {
        operationRef.current += 1;
        pendingPlayRef.current = null;
    }, []);
    const isCurrentEvent = useCallback((event: VideoEvent) => mountedRef.current
        && event.currentTarget === videoRef.current && event.currentTarget === activeVideoRef.current, []);

    const reconcilePlayback = useCallback(() => {
        if (!mountedRef.current) return;
        const video = videoRef.current;
        setPlaybackRequested(canPlay() && clips.length > 0
            && phaseRef.current !== "error" && phaseRef.current !== "blocked");
        if (!canPlay()) {
            invalidatePlay();
            video?.pause();
            setPlaying(false);
            return;
        }
        if (!clips.length || phaseRef.current === "error" || phaseRef.current === "blocked") return;
        if (!loadRef.current) {
            loadRef.current = true;
            setShouldLoad(true);
            updatePhase("loading");
            return;
        }
        // metadata 预加载可能不会触发 loadeddata；先请求播放，才能推动浏览器继续加载画面。
        if (!video || video !== activeVideoRef.current
            || (phaseRef.current !== "loading" && phaseRef.current !== "ready")
            || pendingPlayRef.current !== null || !video.paused) return;

        const revision = revisionRef.current;
        const operation = ++operationRef.current;
        pendingPlayRef.current = operation;
        const isCurrentRequest = () => mountedRef.current && videoRef.current === video
            && activeVideoRef.current === video && revisionRef.current === revision
            && operationRef.current === operation;
        const handleFailure = (error: unknown) => {
            if (!isCurrentRequest()) return;
            pendingPlayRef.current = null;
            setPlaybackRequested(false);
            if (!canPlay()) {
                video.pause();
                return;
            }
            setPlaying(false);
            updatePhase(error instanceof Error && error.name === "NotAllowedError" ? "blocked" : "error");
        };
        try {
            void video.play().then(() => {
                // 换片、离开视口或卸载后，旧 play Promise 不能恢复播放状态。
                if (!isCurrentRequest()) {
                    if (videoRef.current !== video || !canPlay()) video.pause();
                    return;
                }
                pendingPlayRef.current = null;
                if (!canPlay()) {
                    setPlaybackRequested(false);
                    video.pause();
                    return;
                }
                setPlaying(!video.paused);
            }, handleFailure);
        } catch (error) {
            handleFailure(error);
        }
    }, [canPlay, clips.length, invalidatePlay, updatePhase]);

    const beginSelection = useCallback((nextIndex: number) => {
        invalidatePlay();
        // 先解除旧 DOM 的事件资格，再更新 key，防止卸载前的 pause/error 污染新片。
        activeVideoRef.current = null;
        videoRef.current?.pause();
        indexRef.current = nextIndex;
        revisionRef.current += 1;
        loadRef.current = canPlay();
        setPlaying(false);
        setPlaybackRequested(loadRef.current);
        setShouldLoad(loadRef.current);
        updatePhase(loadRef.current ? "loading" : "idle");
        setSelection({ index: nextIndex, revision: revisionRef.current });
    }, [canPlay, invalidatePlay, updatePhase]);

    const select = useCallback((nextIndex: number) => {
        if (!Number.isInteger(nextIndex) || nextIndex < 0 || nextIndex >= clips.length
            || nextIndex === indexRef.current) return;
        beginSelection(nextIndex);
    }, [beginSelection, clips.length]);
    const retry = useCallback(() => {
        if (!clips.length) return;
        intentRef.current = "play";
        beginSelection(indexRef.current);
    }, [beginSelection, clips.length]);
    const togglePlayback = useCallback(() => {
        if (!clips.length) return;
        if (phaseRef.current === "error") {
            retry();
            return;
        }
        if (phaseRef.current === "blocked") {
            intentRef.current = "play";
            updatePhase("ready");
        } else {
            // 加载中也按用户意图切换，不能仅用 video.paused 判断是否要暂停。
            intentRef.current = wantsPlayback() ? "pause" : "play";
        }
        reconcilePlayback();
    }, [clips.length, reconcilePlayback, retry, updatePhase, wantsPlayback]);

    const handleLoaded = useCallback((event: VideoEvent) => {
        if (!isCurrentEvent(event) || event.currentTarget.readyState < HTMLMediaElement.HAVE_CURRENT_DATA
            || phaseRef.current === "error" || phaseRef.current === "blocked") return;
        updatePhase("ready");
        reconcilePlayback();
    }, [isCurrentEvent, reconcilePlayback, updatePhase]);
    const handlePlay = useCallback((event: VideoEvent) => {
        if (!isCurrentEvent(event)) return;
        if (!canPlay() || phaseRef.current === "error" || phaseRef.current === "blocked") {
            event.currentTarget.pause();
            setPlaying(false);
            setPlaybackRequested(false);
            return;
        }
        setPlaying(true);
    }, [canPlay, isCurrentEvent]);
    const handlePause = useCallback((event: VideoEvent) => {
        if (isCurrentEvent(event)) setPlaying(false);
    }, [isCurrentEvent]);
    const handleError = useCallback((event: VideoEvent) => {
        if (!isCurrentEvent(event)) return;
        invalidatePlay();
        updatePhase("error");
        setPlaying(false);
        setPlaybackRequested(false);
        event.currentTarget.pause();
    }, [invalidatePlay, isCurrentEvent, updatePhase]);
    const handleEnded = useCallback((event: VideoEvent) => {
        if (!isCurrentEvent(event)) return;
        setPlaying(false);
        if (!canPlay() || phaseRef.current !== "ready") {
            setPlaybackRequested(false);
            return;
        }
        if (clips.length > 1) {
            beginSelection((indexRef.current + 1) % clips.length);
        } else {
            invalidatePlay();
            event.currentTarget.currentTime = 0;
            reconcilePlayback();
        }
    }, [beginSelection, canPlay, clips.length, invalidatePlay, isCurrentEvent, reconcilePlayback]);

    useLayoutEffect(() => {
        activeVideoRef.current = shouldLoad ? videoRef.current : null;
        reconcilePlayback();
        return () => {
            activeVideoRef.current = null;
            invalidatePlay();
        };
    }, [selection.revision, shouldLoad, invalidatePlay, reconcilePlayback]);

    useEffect(() => {
        mountedRef.current = true;
        const stage = stageRef.current;
        const handleVisibility = () => reconcilePlayback();
        document.addEventListener("visibilitychange", handleVisibility);
        let observer: IntersectionObserver | undefined;
        if (stage && "IntersectionObserver" in window) {
            observer = new IntersectionObserver(([entry]) => {
                visibleRef.current = entry.isIntersecting && entry.intersectionRatio >= 0.08;
                reconcilePlayback();
            }, { root: stage.closest<HTMLElement>(".auth-scene"), threshold: [0, 0.08] });
            observer.observe(stage);
        } else {
            visibleRef.current = true;
            reconcilePlayback();
        }
        return () => {
            mountedRef.current = false;
            visibleRef.current = false;
            activeVideoRef.current = null;
            invalidatePlay();
            observer?.disconnect();
            document.removeEventListener("visibilitychange", handleVisibility);
            videoRef.current?.pause();
        };
    }, [invalidatePlay, reconcilePlayback]);

    useEffect(() => {
        autoAllowedRef.current = autoplay && reducedMotion === false;
        reconcilePlayback();
    }, [autoplay, reducedMotion, reconcilePlayback]);

    return {
        stageRef,
        videoRef,
        index: selection.index,
        revision: selection.revision,
        shouldLoad,
        phase,
        playing,
        playbackRequested,
        select,
        retry,
        togglePlayback,
        handleLoaded,
        handlePlay,
        handlePause,
        handleError,
        handleEnded,
    };
}
