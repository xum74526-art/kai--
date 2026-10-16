import { expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

import { parseContributors } from "../src/pages/welcome/contributors-parse";

const identityAvatar = (path: string) => path;

test("contributor parser supports an explicitly supplied HTML table", () => {
    const readme = `## 贡献者与团队
<table><tr><td>
<img src="assets/user-example.jpg" alt="Example">
<strong>Example<br><sub>项目发起者 · 微信：example</sub></strong><br>
<a href="mailto:example@example.com">example@example.com</a><br>
<em>一起完善创作工具。</em>
</td></tr></table>`;
    const people = parseContributors(readme, identityAvatar);

    expect(people).toHaveLength(1);
    expect(people[0]).toMatchObject({
        name: "Example",
        avatar: "assets/user-example.jpg",
        email: "example@example.com",
        wechat: "example",
        isFounder: true,
    });
    expect(people[0]?.signature).toBe("一起完善创作工具。");
    expect(people.filter((person) => person.isFounder)).toHaveLength(1);
});

test("KAI README does not present the upstream team as its own", () => {
    const readme = readFileSync(resolve(import.meta.dir, "../../README.md"), "utf8");
    expect(parseContributors(readme, identityAvatar)).toEqual([]);
    expect(readme).toContain("开源来源与许可");
    expect(readme).not.toContain("ddcat666@126.com");
});

test("welcome contributors still parse markdown pipe tables", () => {
    const markdown = `## 贡献者与团队

| 头像 | 昵称 | 邮箱 | 签名 |
| --- | --- | --- | --- |
| <img src="assets/user-ddcat.jpg" alt="ddCat"> | ddCat<br>项目发起者 · 微信：ddcat0829 | founder@example.com | 把故事搬上银幕 |
| <img src="assets/user-ken.jpg" alt="ken"> | ken | ken@example.com | 走自己的路 |

## 下一章
`;
    expect(parseContributors(markdown, identityAvatar)).toEqual([
        {
            name: "ddCat",
            avatar: "assets/user-ddcat.jpg",
            signature: "把故事搬上银幕",
            email: "founder@example.com",
            wechat: "ddcat0829",
            isFounder: true,
        },
        {
            name: "ken",
            avatar: "assets/user-ken.jpg",
            signature: "走自己的路",
            email: "ken@example.com",
            wechat: undefined,
            isFounder: false,
        },
    ]);
});
