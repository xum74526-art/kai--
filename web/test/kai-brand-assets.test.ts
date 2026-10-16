import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { strFromU8, unzipSync } from "fflate";

const root = new URL("../../", import.meta.url);
const read = (path: string) => Bun.file(new URL(path, root)).text();

describe("KAI repository brand assets", () => {
    test("unused screenshots and community QR codes with upstream branding are retired", async () => {
        for (const file of ["login.png", "create.png", "project-detail.png", "backend.png", "huabu.png", "tldraw-license-key-settings.png", "qq.jpg", "wx.jpg"]) {
            expect(await Bun.file(new URL(`assets/${file}`, root)).exists()).toBe(false);
        }
    });

    test("prototype diagrams embed the original KAI logos instead of the infinity mark", async () => {
        for (const [diagram, logo, count] of [
            ["business-channel-architecture", "logo.svg", 1],
            ["project-canvas-ui-interactions", "logo-dark.svg", 3],
        ] as const) {
            const source = await read(`assets/prototypes/${diagram}.drawio`);
            expect(source).not.toMatch(/影策|北斗|∞/);
            const images = [...source.matchAll(/data:image\/svg\+xml;base64,([A-Za-z0-9+/=]+)/g)];
            expect(images).toHaveLength(count);
            const original = (await read(`web/public/${logo}`)).replaceAll("\r\n", "\n");
            for (const image of images) {
                expect(Buffer.from(image[1], "base64").toString("utf8").replaceAll("\r\n", "\n")).toBe(original);
            }
        }
    });

    test("every distributed plugin is checked inside its ZIP, preserving author credits", async () => {
        const violations: string[] = [];
        let count = 0;
        for await (const file of new Bun.Glob("*.yingce-plugin").scan(fileURLToPath(new URL("plugin-packages/", root)))) {
            const archive = unzipSync(readFileSync(new URL(`plugin-packages/${file}`, root)));
            for (const [name, bytes] of Object.entries(archive)) {
                if (!/\.(json|md|txt|js|html|svg)$/.test(name) || /(?:LICENSE|NOTICE)/i.test(name)) continue;
                const text = strFromU8(bytes)
                    .replaceAll('\\"', '"')
                    .replace(/"author"\s*:\s*"[^"\r\n]*"/g, "")
                    .replace(/^\s*author:.*$/gm, "");
                if (/影策|北斗|beidou|yingce studio/i.test(text)) violations.push(`${file}:${name}`);
            }
            count++;
        }
        expect(count).toBeGreaterThan(100);
        expect(violations).toEqual([]);
        const archive = unzipSync(readFileSync(new URL("plugin-packages/autodl-comfyui.yingce-plugin", root)));
        const manifest = JSON.parse(strFromU8(archive["manifest.json"]));
        expect(manifest.id).toBe("autodl-comfyui");
        expect(manifest.apiVersion).toBe("yingce.plugin/v2");
        expect(manifest.author).toBe("影策社区");
        expect(manifest.documentation).toContain("KAI影统一视频请求");
    });

    test("deployment names follow the existing private environment without changing data mounts", async () => {
        const compose = await read("deploy/simon/compose.yml");
        expect(compose).toContain("name: ${COMPOSE_PROJECT_NAME:?");
        expect(compose.match(/image: \$\{COMPOSE_PROJECT_NAME:\?/g)).toHaveLength(2);
        expect(compose).toContain("source: ${KAI_DATA_PATH:?");
        expect(compose).toContain("create_host_path: false");
        expect(compose).not.toMatch(/beidou|北斗/);
        expect(await read("deploy/simon/.env.example")).toContain("COMPOSE_PROJECT_NAME=kaiying");
        expect(await read("deploy/backup-production.sh")).toContain('archive_prefix="${CANVAS_BACKUP_PREFIX:-kaiying}"');
    });

    test("legacy launcher delegates safely without overwriting public URLs or data configuration", async () => {
        const launcher = await read("scripts/launch-canvas.ps1");
        expect(launcher).toContain("'start-local.ps1') @PSBoundParameters");
        expect(launcher).toContain("[string]$DataDir = $env:CANVAS_BACKEND_DATA_DIR");
        expect(launcher).not.toMatch(/\$env:\w+\s*=|yingce\.cc\.cd|Stop-Process|InitializeData\s*=\s*\$true/);
        expect(launcher).toContain("$listeningPorts -contains 3000 -and $listeningPorts -contains 8080");
    });

    test("user-facing export names use KAI while data format identifiers stay unchanged", async () => {
        expect(await read("web/src/pages/inspirations/index.tsx")).toContain("`kaiying-inspirations-${");
        expect(await read("web/src/services/diagnostics/diagnostics-api.ts")).toContain('|| "kaiying-diagnostics.zip"');
        expect(await read("web/src/pages/assets/asset-transfer.ts")).toContain('app: "yingce"');
    });
});
