export function normalizeAppBasePath(value?: string) {
    const trimmed = String(value || "/").trim();
    if (!trimmed || trimmed === "." || trimmed === "./") return "/";
    const path = trimmed.replace(/^[./]+/, "").replace(/^\/+|\/+$/g, "");
    return path ? `/${path}/` : "/";
}

export function appRouterBasenameFor(basePath?: string) {
    const normalized = normalizeAppBasePath(basePath);
    return normalized === "/" ? "/" : normalized.slice(0, -1);
}

export function appPathFor(path: string, basePath?: string) {
    if (/^(?:[a-z][a-z\d+.-]*:|\/\/)/i.test(path)) return path;
    const normalizedBase = normalizeAppBasePath(basePath);
    const normalizedPath = path.replace(/^\/+/, "");
    const baseWithoutSlashes = normalizedBase.replace(/^\/+|\/+$/g, "");
    if (baseWithoutSlashes && (normalizedPath === baseWithoutSlashes || normalizedPath.startsWith(`${baseWithoutSlashes}/`))) return `/${normalizedPath}`;
    return normalizedPath ? `${normalizedBase}${normalizedPath}` : normalizedBase;
}

export function apiBaseURLFor(explicitBaseURL: string | undefined, basePath?: string) {
    const configured = String(explicitBaseURL || "").trim();
    if (configured) return configured.replace(/\/+$/, "");
    return appPathFor("api", basePath).replace(/\/+$/, "");
}

export function stripAppBasePath(pathname: string, basePath?: string) {
    const basename = appRouterBasenameFor(basePath);
    if (basename === "/") return pathname || "/";
    if (pathname === basename) return "/";
    return pathname.startsWith(`${basename}/`) ? pathname.slice(basename.length) || "/" : pathname;
}

export function normalizeBackendMediaUrlFor(value: string, basePath?: string) {
    // 后端持久化根路径形式的资源地址，子路径部署时只在读取边界补应用前缀。
    if (!/^\/?api(?:[/?#]|$)/.test(value)) return value;
    return appPathFor(`/${value.replace(/^\/+/, "")}`, basePath);
}

export function normalizeBackendMediaUrlsFor<T>(value: T, basePath?: string): T {
    if (typeof value === "string") return normalizeBackendMediaUrlFor(value, basePath) as T;
    if (Array.isArray(value)) {
        let changed = false;
        const normalized = value.map((item) => {
            const next = normalizeBackendMediaUrlsFor(item, basePath);
            changed ||= next !== item;
            return next;
        });
        return (changed ? normalized : value) as T;
    }
    if (!value || typeof value !== "object") return value;
    const prototype = Object.getPrototypeOf(value);
    if (prototype !== Object.prototype && prototype !== null) return value;

    let changed = false;
    const normalized: Record<string, unknown> = {};
    for (const [key, item] of Object.entries(value)) {
        const next = normalizeBackendMediaUrlsFor(item, basePath);
        changed ||= next !== item;
        normalized[key] = next;
    }
    return (changed ? normalized : value) as T;
}

export function normalizeBackendMediaJsonFor(value: string, basePath?: string) {
    if (!value) return value;
    try {
        const parsed: unknown = JSON.parse(value);
        const normalized = normalizeBackendMediaUrlsFor(parsed, basePath);
        return normalized === parsed ? value : JSON.stringify(normalized);
    } catch {
        return value;
    }
}
