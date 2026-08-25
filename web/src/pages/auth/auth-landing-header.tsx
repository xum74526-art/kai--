import { useEffect, useRef, useState, type KeyboardEvent } from "react";
import { AudioLines, ChevronDown, Film, Moon, Sun, TextCursorInput } from "lucide-react";
import { Link, useLocation } from "react-router";

import { AnimatedThemeToggler } from "@/components/ui/animated-theme-toggler";
import { KaiyingBrandLogo } from "@/components/ui/kaiying-brand-logo";
import type { ThemeName } from "@/stores/use-theme-store";
import { apiOptions, type ApiKind } from "./auth-landing-data";

const optionIcons = {
    text: TextCursorInput,
    audio: AudioLines,
    video: Film,
} satisfies Record<ApiKind, typeof Film>;

type AuthLandingHeaderProps = {
    scrolled: boolean;
    theme: ThemeName;
    selectedApi: ApiKind;
    onApiChange: (api: ApiKind) => void;
    onThemeChange: (theme: ThemeName) => void;
};

export function AuthLandingHeader({ scrolled, theme, selectedApi, onApiChange, onThemeChange }: AuthLandingHeaderProps) {
    const location = useLocation();
    const pickerRef = useRef<HTMLDivElement>(null);
    const triggerRef = useRef<HTMLButtonElement>(null);
    const optionRefs = useRef<Array<HTMLButtonElement | null>>([]);
    const [open, setOpen] = useState(false);
    const selectedOption = apiOptions.find((option) => option.value === selectedApi) ?? apiOptions[2];

    useEffect(() => {
        if (!open) return;
        const closeOnOutsidePointer = (event: PointerEvent) => {
            if (!pickerRef.current?.contains(event.target as Node)) setOpen(false);
        };
        document.addEventListener("pointerdown", closeOnOutsidePointer, true);
        return () => document.removeEventListener("pointerdown", closeOnOutsidePointer, true);
    }, [open]);

    const focusOption = (index: number) => {
        optionRefs.current[index]?.focus();
    };

    const openWithFocus = (fromEnd = false) => {
        setOpen(true);
        window.requestAnimationFrame(() => {
            const selectedIndex = apiOptions.findIndex((option) => option.value === selectedApi);
            focusOption(fromEnd ? apiOptions.length - 1 : Math.max(0, selectedIndex));
        });
    };

    const handleTriggerKeyDown = (event: KeyboardEvent<HTMLButtonElement>) => {
        if (event.key === "ArrowDown" || event.key === "ArrowUp") {
            event.preventDefault();
            openWithFocus(event.key === "ArrowUp");
        } else if (event.key === "Escape" && open) {
            event.preventDefault();
            event.stopPropagation();
            setOpen(false);
        }
    };

    const handleOptionsKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
        if (event.key === "Escape") {
            event.preventDefault();
            event.stopPropagation();
            setOpen(false);
            triggerRef.current?.focus();
            return;
        }
        const currentIndex = optionRefs.current.findIndex((option) => option === document.activeElement);
        let nextIndex: number | undefined;
        if (event.key === "ArrowDown") nextIndex = currentIndex < 0 ? 0 : (currentIndex + 1) % apiOptions.length;
        if (event.key === "ArrowUp") nextIndex = currentIndex < 0 ? apiOptions.length - 1 : (currentIndex - 1 + apiOptions.length) % apiOptions.length;
        if (event.key === "Home") nextIndex = 0;
        if (event.key === "End") nextIndex = apiOptions.length - 1;
        if (nextIndex === undefined) return;
        event.preventDefault();
        focusOption(nextIndex);
    };

    return (
        <header className={`site-header${scrolled ? " is-scrolled" : ""}`}>
            <a className="brand" href="#top" aria-label="KAI影首页">
                <KaiyingBrandLogo decorative />
            </a>

            <nav className="site-nav" aria-label="页面导航">
                <a href="#worlds">故事镜头</a>
                <a href="#create">开始创作</a>
                <a href="#gallery">视觉素材</a>
            </nav>

            <div className="header-actions">
                <div
                    className={`api-picker${open ? " is-open" : ""}`}
                    data-api-value={selectedApi}
                    ref={pickerRef}
                    onKeyDownCapture={(event) => {
                        if (event.key !== "Tab") return;
                        window.requestAnimationFrame(() => {
                            if (!pickerRef.current?.contains(document.activeElement)) setOpen(false);
                        });
                    }}
                >
                    <button
                        ref={triggerRef}
                        className="api-picker-trigger"
                        type="button"
                        aria-expanded={open}
                        aria-controls="model-api-panel"
                        aria-label={`模型 API，当前选择${selectedOption.label} API`}
                        onClick={() => setOpen((value) => !value)}
                        onKeyDown={handleTriggerKeyDown}
                    >
                        <span className="api-picker-label">模型 API</span>
                        <span className="api-picker-current">{selectedOption.label}</span>
                        <span className="api-picker-short" aria-hidden="true">API</span>
                        <ChevronDown className="api-picker-chevron" aria-hidden="true" />
                    </button>

                    <div
                        className="api-picker-panel"
                        id="model-api-panel"
                        role="group"
                        aria-label="选择模型 API"
                        hidden={!open}
                        onKeyDown={handleOptionsKeyDown}
                    >
                        {apiOptions.map((option, index) => {
                            const Icon = optionIcons[option.value];
                            const active = selectedApi === option.value;
                            return (
                                <button
                                    key={option.value}
                                    ref={(element) => { optionRefs.current[index] = element; }}
                                    className="api-picker-option"
                                    type="button"
                                    aria-pressed={active}
                                    onClick={() => {
                                        onApiChange(option.value);
                                        setOpen(false);
                                        triggerRef.current?.focus();
                                    }}
                                >
                                    <span className="api-picker-option-icon" aria-hidden="true"><Icon size={19} strokeWidth={1.8} /></span>
                                    <span className="api-picker-option-copy"><strong>{option.title}</strong><small>{option.description}</small></span>
                                    <span className="api-picker-option-state" aria-hidden="true">{active ? "已选" : ""}</span>
                                </button>
                            );
                        })}
                    </div>
                </div>

                <AnimatedThemeToggler
                    className="icon-button"
                    theme={theme}
                    targetTheme={theme === "dark" ? "light" : "dark"}
                    onThemeChange={onThemeChange}
                    aria-label={theme === "dark" ? "切换为浅色模式" : "切换为深色模式"}
                >
                    {theme === "dark" ? <Sun size={18} /> : <Moon size={18} />}
                </AnimatedThemeToggler>
                {location.pathname === "/register" ? (
                    <Link className="button button-light button-small" to={{ pathname: "/login", search: location.search }}>登录</Link>
                ) : (
                    <a className="button button-light button-small" href="#login">登录</a>
                )}
            </div>
        </header>
    );
}
