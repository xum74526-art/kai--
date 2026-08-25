import {
    useCallback,
    useEffect,
    useRef,
    useState,
    type SyntheticEvent,
} from "react";
import { useReducedMotion } from "motion/react";

export type AuthMediaScene = {
    clip: string;
    poster: string;
    title: string;
    playTo?: number;
};

type MediaPhase = "loading" | "ready" | "error";

type LoadRequest = {
    index: number;
    token: number;
};

export function useAuthMediaSwitcher<T extends AuthMediaScene>(
    scenes: readonly T[],
    { autoAdvance = false }: { autoAdvance?: boolean } = {},
) {
    const stageRef = useRef<HTMLDivElement>(null);
    const videoRef = useRef<HTMLVideoElement>(null);
    const generationRef = useRef(0);
    const committedIndexRef = useRef(0);
    const visibleRef = useRef(false);
    const manuallyPausedRef = useRef(false);
    const boundaryLockedRef = useRef(false);
    const reducedMotion = Boolean(useReducedMotion());
    const reducedMotionRef = useRef(reducedMotion);
    const phaseRef = useRef<MediaPhase>("loading");
    const [loadRequest, setLoadRequest] = useState<LoadRequest>({ index: 0, token: 0 });
    const [committedIndex, setCommittedIndex] = useState(0);
    const [pendingIndex, setPendingIndex] = useState<number | null>(0);
    const [phase, setPhase] = useState<MediaPhase>("loading");
    const [playing, setPlaying] = useState(false);
    const [shouldLoad, setShouldLoad] = useState(false);

    const playCurrent = useCallback((userInitiated = false) => {
        const video = videoRef.current;
        if (
            !video
            || phaseRef.current !== "ready"
            || !visibleRef.current
            || document.hidden
            || (reducedMotionRef.current && !userInitiated)
        ) return;
        void video.play().catch(() => setPlaying(false));
    }, []);

    const beginLoad = useCallback((index: number, userInitiated: boolean) => {
        if (!scenes.length) return;
        const nextIndex = (index + scenes.length) % scenes.length;
        const token = generationRef.current + 1;
        generationRef.current = token;
        manuallyPausedRef.current = userInitiated ? false : manuallyPausedRef.current;
        boundaryLockedRef.current = false;
        phaseRef.current = "loading";
        videoRef.current?.pause();
        setPlaying(false);
        setPendingIndex(nextIndex);
        setPhase("loading");
        setLoadRequest({ index: nextIndex, token });
    }, [scenes.length]);

    const select = useCallback((index: number) => {
        if (index === loadRequest.index && phase !== "error") return;
        beginLoad(index, true);
    }, [beginLoad, loadRequest.index, phase]);

    const retry = useCallback(() => {
        beginLoad(pendingIndex ?? committedIndexRef.current, true);
    }, [beginLoad, pendingIndex]);

    const togglePlayback = useCallback(() => {
        const video = videoRef.current;
        if (!video || phaseRef.current === "loading") return;
        if (phaseRef.current === "error") {
            retry();
            return;
        }
        if (video.paused) {
            manuallyPausedRef.current = false;
            playCurrent(true);
        } else {
            manuallyPausedRef.current = true;
            video.pause();
        }
    }, [playCurrent, retry]);

    const handleReady = useCallback((event: SyntheticEvent<HTMLVideoElement>, token: number, index: number) => {
        if (token !== generationRef.current || event.currentTarget.readyState < HTMLMediaElement.HAVE_CURRENT_DATA) return;
        if (phaseRef.current === "ready" && committedIndexRef.current === index) return;
        committedIndexRef.current = index;
        boundaryLockedRef.current = false;
        phaseRef.current = "ready";
        setCommittedIndex(index);
        setPendingIndex(null);
        setPhase("ready");
        if (!manuallyPausedRef.current) playCurrent(false);
    }, [playCurrent]);

    const handleError = useCallback((token: number, index: number) => {
        if (token !== generationRef.current) return;
        phaseRef.current = "error";
        setPendingIndex(index);
        setPhase("error");
        setPlaying(false);
    }, []);

    const finishCurrentScene = useCallback(() => {
        if (boundaryLockedRef.current || phaseRef.current !== "ready") return;
        const video = videoRef.current;
        if (!video) return;

        boundaryLockedRef.current = true;
        if (autoAdvance && !manuallyPausedRef.current) {
            beginLoad(committedIndexRef.current + 1, false);
            return;
        }

        const resumeFromStart = () => {
            boundaryLockedRef.current = false;
            if (!manuallyPausedRef.current) playCurrent(false);
        };
        video.addEventListener("seeked", resumeFromStart, { once: true });
        video.currentTime = 0;
    }, [autoAdvance, beginLoad, playCurrent]);

    const handleTimeUpdate = useCallback((event: SyntheticEvent<HTMLVideoElement>) => {
        const playTo = scenes[committedIndexRef.current]?.playTo;
        if (typeof playTo !== "number" || event.currentTarget.currentTime < playTo) return;
        finishCurrentScene();
    }, [finishCurrentScene, scenes]);

    const handleEnded = useCallback(() => {
        finishCurrentScene();
    }, [finishCurrentScene]);

    useEffect(() => {
        const stage = stageRef.current;
        if (!stage || !("IntersectionObserver" in window)) {
            visibleRef.current = true;
            setShouldLoad(true);
            return;
        }
        const scrollRoot = stage.closest<HTMLElement>(".auth-landing");
        const loadObserver = new IntersectionObserver(([entry]) => {
            if (entry.isIntersecting) setShouldLoad(true);
        }, { root: scrollRoot, rootMargin: "240px 0px", threshold: 0 });
        const visibilityObserver = new IntersectionObserver(([entry]) => {
            const visible = entry.isIntersecting && entry.intersectionRatio >= 0.08;
            visibleRef.current = visible;
            if (!visible) videoRef.current?.pause();
            else if (!manuallyPausedRef.current) playCurrent(false);
        }, { root: scrollRoot, threshold: [0, 0.08, 0.35] });
        loadObserver.observe(stage);
        visibilityObserver.observe(stage);
        return () => {
            loadObserver.disconnect();
            visibilityObserver.disconnect();
        };
    }, [playCurrent]);

    useEffect(() => {
        const handleVisibility = () => {
            if (document.hidden) videoRef.current?.pause();
            else if (!manuallyPausedRef.current) playCurrent(false);
        };
        document.addEventListener("visibilitychange", handleVisibility);
        return () => document.removeEventListener("visibilitychange", handleVisibility);
    }, [playCurrent]);

    useEffect(() => {
        reducedMotionRef.current = reducedMotion;
        if (reducedMotion) videoRef.current?.pause();
        else if (!manuallyPausedRef.current) playCurrent(false);
    }, [playCurrent, reducedMotion]);

    return {
        stageRef,
        videoRef,
        loadRequest,
        committedIndex,
        pendingIndex,
        phase,
        playing,
        shouldLoad,
        select,
        retry,
        togglePlayback,
        handleReady,
        handleError,
        handleTimeUpdate,
        handleEnded,
        handlePlay: () => setPlaying(true),
        handlePause: () => setPlaying(false),
    };
}
