import { ArrowUpRight, Code2 } from "lucide-react";

import { CometCard } from "@/components/ui/aceternity/comet-card";
import { useAppearanceStore } from "@/stores/use-appearance-store";

const repository = "https://github.com/xum74526-art/kai--";

export function WelcomeContributorsCard() {
    const brandName = useAppearanceStore((state) => state.appearance.brandName);

    return (
        <section id="contributors" className="welcome-contributors" aria-labelledby="welcome-contributors-title">
            <CometCard containerClassName="welcome-contributors-perspective" className="welcome-contributors-card" rotateDepth={3.5} translateDepth={3}>
                <div className="welcome-contributors-heading">
                    <span className="welcome-contributors-kicker">
                        <Code2 size={15} />
                        开源共建
                    </span>
                    <div>
                        <h3 id="welcome-contributors-title">一起完善{brandName}</h3>
                        <p>分享创作建议、反馈使用问题，让工作台更贴近你的创作。</p>
                    </div>
                    <a href={repository + "/issues"} target="_blank" rel="noreferrer" aria-label="向 KAI影仓库反馈建议">
                        反馈建议
                        <ArrowUpRight size={14} />
                    </a>
                </div>
                <a className="welcome-source" href={repository + "/blob/main/CONTRIBUTING.md"} target="_blank" rel="noreferrer">
                    查看贡献指南
                    <ArrowUpRight size={14} />
                </a>
            </CometCard>
        </section>
    );
}
