import { describe, expect, test } from "bun:test";
import { existsSync, readFileSync } from "node:fs";
import { resolve } from "node:path";
import { chapters, getWelcomeLook, showcases, welcomeLooks } from "../src/pages/welcome/story";

const publicFile = (url: string) => resolve(import.meta.dir, "../public", url.replace(/^\//, ""));

describe("welcome story", () => {
    test("uses KAIying and retains user showcase films alongside upstream looks", () => {
        expect(chapters[0].title).toBe("KAI影");
        expect(welcomeLooks.map((look) => look.id)).toEqual(["neon-rain-city", "golden-countryside", "ink-mountain-lake", "spring", "charge", "wing-it"]);
        expect(getWelcomeLook("").id).toBe("neon-rain-city");
        expect(getWelcomeLook("?look=unknown").id).toBe("neon-rain-city");
        for (const look of welcomeLooks) expect(getWelcomeLook(`?look=${look.id}`)).toBe(look);
    });

    test("every reel slot has local media and screenplay", () => {
        for (const look of welcomeLooks) {
            expect(look.frames).toHaveLength(12);
            expect(look.screenplay).toHaveLength(12);
            expect(look.screenplay.every((line) => line.trim().length > 0)).toBe(true);
            for (const frame of look.frames) expect(existsSync(publicFile(frame))).toBe(true);
            if (look.video) expect(existsSync(publicFile(look.video))).toBe(true);
        }
        expect(welcomeLooks.filter((look) => look.video).map((look) => look.id)).toEqual(["neon-rain-city", "golden-countryside", "ink-mountain-lake", "charge"]);
        for (const showcase of showcases) expect(existsSync(publicFile(showcase.image))).toBe(true);
    });

    test("credits cover attributed upstream looks separately from user media and the code license", () => {
        const credits = readFileSync(publicFile("/welcome/credits.html"), "utf8");
        expect(credits).toContain("https://creativecommons.org/licenses/by/4.0/");
        for (const look of welcomeLooks.filter((item) => item.credit)) expect(credits).toContain(`id="${look.id}"`);
        expect(credits).not.toContain("竹影");
        expect(existsSync(publicFile("/welcome/sequence.mp4"))).toBe(false);
    });
});
