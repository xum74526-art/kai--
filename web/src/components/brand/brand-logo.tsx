import { useState, type CSSProperties, type ReactNode } from "react";

import { cn } from "@/lib/utils";
import { appearanceLogoURL, useAppearanceStore } from "@/stores/use-appearance-store";
import type { ThemeName } from "@/stores/use-theme-store";
import { useActiveTheme } from "@/stores/canvas/use-canvas-theme-store";

type BrandLogoProps = {
    className?: string;
    fallback: ReactNode;
    alt?: string;
    theme?: ThemeName | "auto";
    variant?: "symbol" | "wordmark";
};

export function BrandLogo({ className, fallback, alt = "", theme = "auto", variant = "symbol" }: BrandLogoProps) {
    const appearance = useAppearanceStore((state) => state.appearance);
    const currentTheme = useActiveTheme();
    const resolvedTheme = theme === "auto" ? currentTheme : theme;
    const source = appearance.logoConfigured ? appearanceLogoURL(appearance, resolvedTheme) : resolvedTheme === "dark" ? "/logo-dark.svg" : "/logo.svg";
    const [failedSource, setFailedSource] = useState<string | null>(null);
    if (!appearance.logoConfigured && variant === "symbol") {
        if (failedSource === source) return <>{fallback}</>;
        // 新版导航使用方形标记与文字组合，截取原字标中的镜头图形，保持既有品牌比例。
        return (
            <svg className={cn("block", className)} viewBox="210 0 130 128" role={alt ? "img" : undefined} aria-label={alt || undefined} aria-hidden={!alt || undefined}>
                <image href={source} width="350" height="128" onError={() => setFailedSource(source)} />
            </svg>
        );
    }
    // A configured custom logo must never fall through to the built-in brand
    // when its file becomes unavailable. Keep its footprint neutral instead.
    if (failedSource === source) return appearance.logoConfigured ? <span className={cn("block", className)} aria-hidden="true" /> : <>{fallback}</>;
    return (
        <img
            src={source}
            alt={alt}
            className={cn("block object-contain", className)}
            draggable={false}
            onError={(event) => {
                event.currentTarget.style.visibility = "hidden";
                setFailedSource(source);
            }}
        />
    );
}

export function BrandLogoFrame({ className, logoClassName, fallback, alt = "", theme = "auto" }: BrandLogoProps & { logoClassName?: string }) {
    const frameEnabled = useAppearanceStore((state) => state.appearance.logoFrameEnabled);
    const unframedStyle: CSSProperties | undefined = frameEnabled
        ? undefined
        : {
              background: "transparent",
              borderColor: "transparent",
              borderRadius: 0,
              boxShadow: "none",
              color: "inherit",
          };
    return (
        <span className={cn("brand-logo-frame", className)} data-logo-frame-enabled={frameEnabled} style={unframedStyle}>
            <BrandLogo className={logoClassName} fallback={fallback} alt={alt} theme={theme} />
        </span>
    );
}
