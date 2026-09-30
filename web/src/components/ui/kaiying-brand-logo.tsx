import { publicAssetPath } from "@/lib/app-base-path";
import { cn } from "@/lib/utils";

type KaiyingBrandLogoProps = {
    className?: string;
    compact?: boolean;
    decorative?: boolean;
};

export function KaiyingBrandLogo({ className, compact = false, decorative = false }: KaiyingBrandLogoProps) {
    return (
        <span
            className={cn("kaiying-brand-logo", compact && "is-compact", className)}
            role={decorative ? undefined : "img"}
            aria-label={decorative ? undefined : "KAI影"}
            aria-hidden={decorative || undefined}
        >
            <img className="kaiying-brand-logo-light" src={publicAssetPath("logo.svg")} alt="" />
            <img className="kaiying-brand-logo-dark" src={publicAssetPath("logo-dark.svg")} alt="" />
        </span>
    );
}
