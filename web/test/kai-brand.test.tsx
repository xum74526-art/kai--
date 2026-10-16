import { describe, expect, test } from "bun:test";
import { fileURLToPath } from "node:url";
import { renderToStaticMarkup } from "react-dom/server";
import { BrandLogo } from "../src/components/brand/brand-logo";
import { DEFAULT_PUBLIC_APPEARANCE, normalizePublicAppearance } from "../src/stores/use-appearance-store";
import { showcases } from "../src/pages/welcome/story";

describe("KAI brand", () => {
    test("repository entry points belong to KAI without changing protocol identifiers", async () => {
        const root = new URL("../../", import.meta.url);
        const read = (file: string) => Bun.file(new URL(file, root)).text();
        const readme = await read("README.md");
        expect(readme).toContain('<h1 align="center">KAI影</h1>');
        expect(readme).toContain("xum74526-art/kai--/main/scripts/install-server.sh");
        expect(readme).not.toMatch(/影策|北斗|yingce\.tv|## 赞助商|## 贡献者与团队/);
        expect(readme).toContain("yingce.plugin/v2");
        expect(readme).toContain("[MIT 许可证](LICENSE)");
        for (const path of ["SECURITY.md", ".github/ISSUE_TEMPLATE/config.yml", "scripts/install-server.sh", "scripts/install-host-updater.sh", "scripts/install-server-image.sh", "web/src/pages/admin/settings/system-update-page.tsx", "web/src/pages/welcome/contributors-card.tsx"]) {
            const source = await read(path);
            expect(source).toContain("xum74526-art/kai--");
            expect(source).not.toContain("ddcat-ai/open-ai-canvas");
        }
        expect(await read("docker-compose.deploy.yml")).toContain("ghcr.io/xum74526-art/open-ai-canvas-yingce-agent");
        const community = await read("web/src/pages/welcome/contributors-card.tsx");
        expect(community).not.toContain("welcomeContributors");
        expect(community).not.toContain("项目发起者");
    });

    test("bundled logos and favicon keep the original KAI identity", async () => {
        for (const asset of ["logo.svg", "logo-dark.svg", "favicon.svg"]) {
            const source = await Bun.file(new URL(`../public/${asset}`, import.meta.url)).text();
            expect(source).toContain('<title id="title">KAI影</title>');
            expect(source).not.toMatch(/影策|北斗/);
        }
        expect(DEFAULT_PUBLIC_APPEARANCE.brandName).toBe("KAI影");
        expect(DEFAULT_PUBLIC_APPEARANCE.canvas.agentName).toBe("KAI影");
    });

    test("wordmarks and compact marks use the matching original logo in both themes", () => {
        for (const theme of ["light", "dark"] as const) {
            const source = theme === "dark" ? "/logo-dark.svg" : "/logo.svg";
            const wordmark = renderToStaticMarkup(<BrandLogo theme={theme} variant="wordmark" alt="KAI影" fallback={<span>K</span>} />);
            const symbol = renderToStaticMarkup(<BrandLogo theme={theme} variant="symbol" alt="KAI影" fallback={<span>K</span>} />);
            expect(wordmark).toContain(`src="${source}"`);
            expect(wordmark).toContain('alt="KAI影"');
            expect(symbol).toContain(`href="${source}"`);
            expect(symbol).toContain('viewBox="210 0 130 128"');
        }
    });

    test("brand changes do not discard administrator appearance settings", () => {
        const appearance = normalizePublicAppearance({
            brandName: "自定义工作室",
            logoConfigured: true,
            logoUrl: "/api/public/appearance/assets/logo?v=custom",
            authVideoConfigured: true,
            authVideoUrl: "/api/public/appearance/assets/video?v=custom",
            authVideoAutoplay: false,
        });
        expect(appearance.brandName).toBe("自定义工作室");
        expect(appearance.logoConfigured).toBe(true);
        expect(appearance.logoUrl).toContain("v=custom");
        expect(appearance.authVideoUrl).toContain("v=custom");
        expect(appearance.authVideoAutoplay).toBe(false);
    });

    test("product copy contains no upstream branding, apart from author attribution", async () => {
        const root = new URL("../src/", import.meta.url);
        const violations: string[] = [];
        for await (const file of new Bun.Glob("**/*.{ts,tsx,css}").scan(fileURLToPath(root))) {
            const source = await Bun.file(new URL(file, root)).text();
            source.split(/\r?\n/).forEach((line, index) => {
                if (/影策|北斗/.test(line) && !/^\s*author:/.test(line)) violations.push(`${file}:${index + 1}`);
            });
        }
        expect(violations).toEqual([]);
    });

    test("navigation fallbacks no longer use the upstream infinity mark", async () => {
        for (const file of ["components/layout/workspace-sidebar-nav.tsx", "pages/admin/components/admin-shell.tsx"]) {
            const source = await Bun.file(new URL(`../src/${file}`, import.meta.url)).text();
            expect(source).toContain("<BrandLogoFrame");
            expect(source).not.toContain("InfinityIcon");
        }
    });

    test("welcome retires screenshots with baked-in upstream logos", async () => {
        expect(showcases.find((item) => item.name === "项目工作台")?.image).toBeNull();
        for (const item of showcases) {
            if (!item.image) continue;
            expect(item.image).not.toMatch(/workbench-(create|project)\.webp/);
            expect(await Bun.file(new URL(`../public${item.image}`, import.meta.url)).exists()).toBe(true);
        }
        const page = await Bun.file(new URL("../src/pages/welcome/index.tsx", import.meta.url)).text();
        expect(page).toContain('variant="wordmark" className="welcome-brand-logo"');
        expect(page).toContain('variant="wordmark" className="welcome-workbench-brand-logo"');
    });
});
