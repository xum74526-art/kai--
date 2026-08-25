import { useEffect, useRef, useState, type KeyboardEvent } from "react";
import { useReducedMotion } from "motion/react";
import { Outlet, useLocation, useNavigate } from "react-router";

import { KaiyingBrandLogo } from "@/components/ui/kaiying-brand-logo";
import { publicAssetPath } from "@/lib/app-base-path";
import { useThemeStore } from "@/stores/use-theme-store";
import { ClosingSection, CreationLabSection, ShowcaseSection } from "./auth-creation-sections";
import type { ApiKind } from "./auth-landing-data";
import { AuthLandingHeader } from "./auth-landing-header";
import { AuthShowreel } from "./auth-showreel";
import { StoryWorldsSection } from "./auth-story-sections";
import shellStyles from "./auth-landing-shell.css?inline";
import heroStyles from "./auth-landing-hero.css?inline";
import sectionStyles from "./auth-landing-sections.css?inline";
import responsiveStyles from "./auth-landing-responsive.css?inline";
import integrationStyles from "./auth-landing-integration.css?inline";

const AUTH_STYLES = [shellStyles, heroStyles, sectionStyles, responsiveStyles, integrationStyles].join("\n");
const AUTH_TABS = [
    { key: "login", label: "登录", path: "/login" },
    { key: "register", label: "注册", path: "/register" },
] as const;

export function LinuxDOIcon() {
    return <img aria-hidden="true" src={publicAssetPath("icons/linuxdo.svg")} alt="" />;
}

export function AuthScene() {
    const location = useLocation();
    const navigate = useNavigate();
    const rootRef = useRef<HTMLDivElement>(null);
    const reducedMotion = Boolean(useReducedMotion());
    const theme = useThemeStore((state) => state.theme);
    const setTheme = useThemeStore((state) => state.setTheme);
    const [headerScrolled, setHeaderScrolled] = useState(false);
    const [selectedApi, setSelectedApi] = useState<ApiKind>("video");
    const activeTab = location.pathname === "/register" ? "register" : "login";

    useEffect(() => {
        const root = rootRef.current;
        if (!root) return;
        const revealItems = Array.from(root.querySelectorAll<HTMLElement>(".reveal"));
        if (reducedMotion || !("IntersectionObserver" in window)) {
            revealItems.forEach((item) => item.classList.add("is-visible"));
            return;
        }
        const observer = new IntersectionObserver((entries) => {
            entries.forEach((entry) => {
                if (!entry.isIntersecting) return;
                entry.target.classList.add("is-visible");
                observer.unobserve(entry.target);
            });
        }, { root, rootMargin: "0px 0px -8% 0px", threshold: 0.08 });
        revealItems.forEach((item) => observer.observe(item));
        return () => observer.disconnect();
    }, [reducedMotion]);

    useEffect(() => {
        const root = rootRef.current;
        const hero = root?.querySelector(".hero");
        if (!root || !hero || !("IntersectionObserver" in window)) return;
        const observer = new IntersectionObserver(([entry]) => {
            setHeaderScrolled(entry.intersectionRatio < 0.88);
        }, { root, threshold: [0, 0.88, 1] });
        observer.observe(hero);
        return () => observer.disconnect();
    }, []);

    const selectTab = (path: "/login" | "/register") => {
        navigate({ pathname: path, search: location.search });
    };

    const handleTabKeyDown = (event: KeyboardEvent<HTMLButtonElement>, index: number) => {
        let nextIndex: number | undefined;
        if (event.key === "ArrowRight") nextIndex = (index + 1) % AUTH_TABS.length;
        if (event.key === "ArrowLeft") nextIndex = (index - 1 + AUTH_TABS.length) % AUTH_TABS.length;
        if (event.key === "Home") nextIndex = 0;
        if (event.key === "End") nextIndex = AUTH_TABS.length - 1;
        if (nextIndex === undefined) return;
        event.preventDefault();
        const nextTab = AUTH_TABS[nextIndex];
        selectTab(nextTab.path);
        window.requestAnimationFrame(() => document.getElementById(`auth-${nextTab.key}-tab`)?.focus());
    };

    const requestAuth = () => {
        const auth = rootRef.current?.querySelector<HTMLElement>("#login");
        auth?.scrollIntoView({ behavior: reducedMotion ? "auto" : "smooth", block: "center" });
        window.requestAnimationFrame(() => {
            const field = document.getElementById(activeTab === "register" ? "auth-register-username" : "auth-identity");
            field?.focus({ preventScroll: true });
        });
    };

    const selectHeaderApi = (api: ApiKind) => {
        setSelectedApi(api);
        const creation = rootRef.current?.querySelector<HTMLElement>("#create");
        creation?.scrollIntoView({ behavior: reducedMotion ? "auto" : "smooth", block: "start" });
    };

    return (
        <>
            <style>{AUTH_STYLES}</style>
            <div className="auth-landing" data-theme={theme} ref={rootRef}>
                <a className="skip-link" href="#main">跳到主要内容</a>
                <AuthLandingHeader
                    scrolled={headerScrolled}
                    theme={theme}
                    selectedApi={selectedApi}
                    onApiChange={selectHeaderApi}
                    onThemeChange={setTheme}
                />

                <main id="main">
                    <section className={`hero${activeTab === "register" ? " is-register" : ""}`} id="top" aria-labelledby="hero-title">
                        <div className="hero-grid page-shell">
                            <div className="hero-copy reveal">
                                <p className="eyebrow">KAI影 AI 影视创作工作台</p>
                                <h1 id="hero-title">
                                    <span className="hero-title-line">从这一帧，</span>
                                    <span className="hero-title-line">进入下一幕</span>
                                </h1>
                                <p className="hero-intro">登录后，把人物、场景和镜头节奏带进同一块画布。</p>

                                <div className="auth-shell" id="login">
                                    <div className="auth-tabs" role="tablist" aria-label="账号方式">
                                        {AUTH_TABS.map((tab, index) => {
                                            const selected = activeTab === tab.key;
                                            return (
                                                <button
                                                    id={`auth-${tab.key}-tab`}
                                                    type="button"
                                                    role="tab"
                                                    aria-selected={selected}
                                                    aria-controls="auth-panel"
                                                    tabIndex={selected ? 0 : -1}
                                                    key={tab.key}
                                                    onClick={() => selectTab(tab.path)}
                                                    onKeyDown={(event) => handleTabKeyDown(event, index)}
                                                >
                                                    {tab.label}
                                                </button>
                                            );
                                        })}
                                    </div>
                                    <div id="auth-panel" role="tabpanel" aria-labelledby={`auth-${activeTab}-tab`}>
                                        <Outlet />
                                    </div>
                                </div>
                            </div>

                            <AuthShowreel />
                        </div>
                    </section>

                    <StoryWorldsSection />
                    <CreationLabSection
                        selectedApi={selectedApi}
                        onApiChange={setSelectedApi}
                        onRequestAuth={requestAuth}
                    />
                    <ShowcaseSection />
                    <ClosingSection onRequestAuth={requestAuth} />
                </main>

                <footer className="page-shell">
                    <a className="brand footer-brand" href="#top" aria-label="KAI影首页">
                        <KaiyingBrandLogo decorative />
                    </a>
                    <p>把故事、素材和模型带进同一块创作画布。</p>
                    <a href="#top">回到顶部</a>
                </footer>
            </div>
        </>
    );
}
