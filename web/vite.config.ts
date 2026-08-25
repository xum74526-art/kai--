import { dirname, resolve } from "node:path";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import react from "@vitejs/plugin-react";
import { defineConfig, loadEnv } from "vite";

import { normalizeAppBasePath } from "./src/lib/app-base-path-core";

const webDir = dirname(fileURLToPath(import.meta.url));
const appVersion = readFileSync(resolve(webDir, "../VERSION"), "utf8").trim();
const appChangelog = readFileSync(resolve(webDir, "../CHANGELOG.md"), "utf8");
const apiProxyTarget = process.env.VITE_API_PROXY_TARGET?.trim() || "http://127.0.0.1:8080";

export default defineConfig(({ mode }) => {
    const configEnv = loadEnv(mode, webDir, "");
    const base = normalizeAppBasePath(configEnv.VITE_APP_BASE_PATH || process.env.VITE_APP_BASE_PATH);
    const apiPath = `${base === "/" ? "" : base.slice(0, -1)}/api`;
    const oauthPath = `${base === "/" ? "" : base.slice(0, -1)}/oauth/linuxdo/callback`;
    return {
        base,
        plugins: [react()],
        define: {
            __APP_VERSION__: JSON.stringify(appVersion),
            __APP_CHANGELOG__: JSON.stringify(appChangelog),
        },
        server: {
            proxy: {
                [apiPath]: {
                    target: apiProxyTarget,
                    changeOrigin: true,
                    xfwd: true,
                    rewrite: base === "/" ? undefined : (path: string) => path.slice(base.length - 1),
                },
                [oauthPath]: {
                    target: apiProxyTarget,
                    changeOrigin: true,
                    xfwd: true,
                    rewrite: base === "/" ? undefined : () => "/oauth/linuxdo/callback",
                },
            },
        },
        resolve: {
            alias: {
                "@": resolve(webDir, "src"),
            },
        },
    };
});
