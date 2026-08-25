import { apiBaseURLFor, appPathFor, appRouterBasenameFor, normalizeAppBasePath, normalizeBackendMediaJsonFor, normalizeBackendMediaUrlFor, normalizeBackendMediaUrlsFor, stripAppBasePath } from "@/lib/app-base-path-core";

export const appBasePath = normalizeAppBasePath(import.meta.env.BASE_URL);
export const appRouterBasename = appRouterBasenameFor(appBasePath);

export function appPath(path: string) {
    return appPathFor(path, appBasePath);
}

export function publicAssetPath(path: string) {
    return appPathFor(path, appBasePath);
}

export function configuredApiBaseURL(explicitBaseURL?: string) {
    return apiBaseURLFor(explicitBaseURL, appBasePath);
}

export function appRoutePath(pathname: string) {
    return stripAppBasePath(pathname, appBasePath);
}

export function normalizeBackendMediaUrl(value: string) {
    return normalizeBackendMediaUrlFor(value, appBasePath);
}

export function normalizeBackendMediaUrls<T>(value: T) {
    return normalizeBackendMediaUrlsFor(value, appBasePath);
}

export function normalizeBackendMediaJson(value: string) {
    return normalizeBackendMediaJsonFor(value, appBasePath);
}
