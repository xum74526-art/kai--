import { scopedLocalStorage } from "@/lib/user-scope";

export type SystemStyleLibraryPreference = "favorites" | "recent";

const storageKeys: Record<SystemStyleLibraryPreference, string> = {
    favorites: "canvas:style-library:favorites",
    recent: "canvas:style-library:recent",
};

// 系统风格本身是公开的，但收藏和最近使用属于账号偏好，必须跟随当前用户 scope。
export function readSystemStyleLibraryIds(preference: SystemStyleLibraryPreference) {
    try {
        const parsed = JSON.parse(scopedLocalStorage.getItem(storageKeys[preference]) || "[]");
        return Array.isArray(parsed) ? parsed.filter((value): value is string => typeof value === "string").slice(0, 100) : [];
    } catch {
        return [];
    }
}

export function persistSystemStyleLibraryIds(preference: SystemStyleLibraryPreference, ids: string[]) {
    scopedLocalStorage.setItem(storageKeys[preference], JSON.stringify(ids));
}
