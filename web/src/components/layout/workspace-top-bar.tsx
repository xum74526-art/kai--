import { PanelLeftClose, PanelLeftOpen, Settings } from "lucide-react";
import { Link, useLocation } from "react-router";

import { SystemAnnouncementCenter } from "@/components/layout/system-announcement-center";
import { AnimatedThemeToggler } from "@/components/ui/animated-theme-toggler";
import { KaiyingBrandLogo } from "@/components/ui/kaiying-brand-logo";
import { navigationTools } from "@/constant/navigation-tools";
import { useThemeStore } from "@/stores/use-theme-store";
import { useUserStore } from "@/stores/use-user-store";

export function WorkspaceTopBar({ sidebarOpen, onToggleSidebar }: { sidebarOpen: boolean; onToggleSidebar: () => void }) {
    const theme = useThemeStore((state) => state.theme);
    const setTheme = useThemeStore((state) => state.setTheme);
    const user = useUserStore((state) => state.user);
    const { pathname } = useLocation();
    const slug = pathname.split("/").filter(Boolean)[0];
    const activeTool = navigationTools.find((tool) => tool.slug === slug);
    const pageTitle = !slug || slug === "home" ? "创作台" : slug === "projects" ? "项目" : activeTool?.label || "创作台";

    return (
        <header className="app-workspace-topbar flex shrink-0 items-center justify-between gap-3 px-4 sm:px-5">
            <div className="app-workspace-topbar-leading flex min-w-0 items-center gap-2">
                <button type="button" className="app-workspace-topbar-icon-button" aria-label={sidebarOpen ? "收起侧栏" : "展开侧栏"} onClick={onToggleSidebar}>
                    {sidebarOpen ? <PanelLeftClose className="size-4" /> : <PanelLeftOpen className="size-4" />}
                </button>
                <Link to="/home" className="app-workspace-brand-link inline-flex min-w-0 items-center text-foreground" aria-label="北斗 KAI影工作台" title="北斗 KAI影工作台">
                    <KaiyingBrandLogo compact decorative />
                </Link>
                <span className="app-workspace-topbar-divider" aria-hidden />
                <strong className="app-workspace-page-context truncate">{pageTitle}</strong>
            </div>
            <div className="flex shrink-0 items-center gap-1">
                {user ? <SystemAnnouncementCenter userId={user.id} className="app-workspace-topbar-icon-button" /> : null}
                <AnimatedThemeToggler className="app-workspace-topbar-icon-button" theme={theme} onThemeChange={setTheme} aria-label="切换主题" />
                <Link to="/settings" className="app-workspace-topbar-icon-button" aria-label="设置" title="设置">
                    <Settings className="size-4" />
                </Link>
            </div>
        </header>
    );
}
