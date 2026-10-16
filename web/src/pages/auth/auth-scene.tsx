import { motion, useReducedMotion } from "motion/react";
import { Tabs } from "antd";
import { ArrowLeft } from "lucide-react";
import { Link, Outlet, useLocation, useNavigate } from "react-router";

import { BrandLogo } from "@/components/brand/brand-logo";
import { SiteComplianceFooter } from "@/components/layout/site-compliance-footer";
import { aceternityMotion } from "@/lib/aceternity-motion";
import { DEFAULT_PUBLIC_APPEARANCE, useAppearanceStore } from "@/stores/use-appearance-store";
import { useThemeStore } from "@/stores/use-theme-store";
import { AuthShowcase } from "./auth-showcase";
import "./auth-showcase.css";

const AUTH_TABS = [
    { key: "login", label: "登录" },
    { key: "register", label: "注册" },
];

const authCopy = {
    login: {
        title: "进入创作现场",
        description: "继续编辑你的画布、素材与生成任务。",
    },
    register: {
        title: "建立你的创作空间",
        description: "一个账号管理画布、素材、技能和模型偏好。",
    },
    recovery: {
        title: "重新设置密码",
        description: "验证账号邮箱后，设置一个新的登录密码。",
    },
} as const;

export function LinuxDOIcon() {
    return (
        <span
            aria-hidden
            className="size-5 shrink-0 rounded-full"
            style={{
                background: "linear-gradient(to bottom, #1d1d1f 0 33.333%, #efefef 33.333% 66.666%, #feb005 66.666% 100%)",
                boxShadow: "0 0 0 1px rgba(255,255,255,.14)",
            }}
        />
    );
}

export function AuthScene() {
    const appearance = useAppearanceStore((state) => state.appearance);
    const appearanceResolved = useAppearanceStore((state) => state.resolved);
    const theme = useThemeStore((state) => state.theme);
    const location = useLocation();
    const navigate = useNavigate();
    const reducedMotion = useReducedMotion();
    const recovery = location.pathname === "/forgot-password";
    const activeTab = location.pathname === "/register" ? "register" : "login";
    const copy = recovery ? authCopy.recovery : activeTab === "register" ? authCopy.register : authCopy.login;
    const showBrandName = appearance.logoConfigured || appearance.brandName !== DEFAULT_PUBLIC_APPEARANCE.brandName;

    return (
        <main className="auth-scene flex h-dvh min-h-0 flex-col overflow-y-auto">
            <header className="flex shrink-0 items-center justify-between gap-4 px-4 py-4 sm:px-6 lg:px-10">
                {appearanceResolved ? (
                    <Link to="/" aria-label={appearance.brandName} className="auth-scene-brand inline-flex min-w-0 items-center gap-3 text-sm font-semibold transition-opacity hover:opacity-80">
                        <BrandLogo theme={theme} variant="wordmark" className="h-10 w-auto max-w-48 object-contain sm:h-12" alt="" fallback={<span>{appearance.brandName}</span>} />
                        {showBrandName ? <span className="truncate">{appearance.brandName}</span> : null}
                    </Link>
                ) : <span className="h-10 w-32" aria-busy="true" />}
                <Link to="/" className="auth-scene-return inline-flex min-h-10 shrink-0 items-center gap-2 rounded-full px-4 text-xs transition">
                    <ArrowLeft className="size-3.5" />
                    返回首页
                </Link>
            </header>
            <div className="auth-scene-layout grid min-w-0 flex-1 grid-cols-1 items-center gap-6 px-4 pb-6 sm:px-6 lg:grid-cols-5 lg:gap-10 lg:px-10">
                <section className="auth-scene-hero order-2 flex min-w-0 flex-col gap-4 lg:order-1 lg:col-span-3 lg:gap-6" aria-label={`${appearance.brandName}品牌影片`}>
                    <motion.div
                        initial={reducedMotion ? false : { opacity: 0, y: 18 }}
                        animate={{ opacity: 1, y: 0 }}
                        transition={{ duration: aceternityMotion.duration.panel, ease: aceternityMotion.easing.enter }}
                        className="auth-scene-copy max-w-2xl"
                    >
                        {appearanceResolved ? <>
                            <h1 className="auth-scene-hero-title max-w-xl whitespace-pre-line text-2xl font-semibold leading-tight sm:text-3xl lg:text-4xl">{appearance.authHeroTitle}</h1>
                            {appearance.authHeroDescription ? <p className="auth-scene-hero-muted mt-3 max-w-xl whitespace-pre-line text-sm leading-6">{appearance.authHeroDescription}</p> : null}
                        </> : <span className="block h-16" aria-busy="true" />}
                    </motion.div>
                    <AuthShowcase />
                </section>

                <section className="auth-scene-form-pane order-1 flex min-w-0 flex-col items-center lg:order-2 lg:col-span-2">
                        <motion.div
                            initial={reducedMotion ? false : { opacity: 0, y: 14 }}
                            animate={{ opacity: 1, y: 0 }}
                            transition={{ duration: aceternityMotion.duration.panel, ease: aceternityMotion.easing.enter }}
                            className="w-full max-w-md"
                        >
                            <div className="auth-scene-card overflow-hidden rounded-lg">
                                <section aria-label={copy.title} className="flex flex-col">
                                    <header className="auth-scene-card-header px-5 pb-4 pt-5 sm:px-6 sm:pt-6">
                                        <h2 className="text-2xl font-semibold">{copy.title}</h2>
                                        <p className="auth-scene-muted text-sm leading-6">{copy.description}</p>
                                    </header>
                                    {!recovery ? (
                                        <div className="px-5 sm:px-6">
                                            <Tabs className="auth-card-tabs" activeKey={activeTab} items={AUTH_TABS} onChange={(key) => navigate({ pathname: key === "register" ? "/register" : "/login", search: location.search })} />
                                        </div>
                                    ) : null}
                                    <div key={location.pathname} className="px-5 pb-5 pt-4 sm:px-6 sm:pb-6">
                                        <Outlet />
                                    </div>
                                </section>
                            </div>
                        </motion.div>
                    <SiteComplianceFooter variant="auth" className="relative inset-auto mt-6 w-full max-w-md shrink-0" />
                </section>
            </div>
        </main>
    );
}
