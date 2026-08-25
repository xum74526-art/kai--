import type { ReactNode } from "react";
import { useEffect, useState } from "react";

import { WorkspaceErrorState } from "@/components/layout/workspace-state";
import { FullScreenLoader } from "@/components/ui/aceternity/full-screen-loader";
import { applyUserSession } from "@/lib/user-session";
import { getAuthSession } from "@/services/api/auth";
import { ApiError } from "@/services/api/request";
import { preloadWorkspaceRoute } from "@/lib/workspace-route-modules";
import { useUserStore } from "@/stores/use-user-store";
import { appRoutePath } from "@/lib/app-base-path";

function isExplicitlyUnauthenticated(error: unknown) {
    return error instanceof ApiError && (error.status === 401 || error.status === 403);
}

function sessionRecoveryErrorMessage(error: unknown) {
    return error instanceof Error && error.message.trim() ? error.message : "无法连接登录服务";
}

export function AuthSessionHydrator({ children }: { children: ReactNode }) {
    const hydrated = useUserStore((state) => state.hydrated);
    const [recoveryError, setRecoveryError] = useState<string | null>(null);
    const [retrySequence, setRetrySequence] = useState(0);

    useEffect(() => {
        let cancelled = false;
        setRecoveryError(null);
        // 登录态与当前工作区 chunk 并行恢复，避免进入应用后再出现一次页面级等待。
        preloadWorkspaceRoute(appRoutePath(window.location.pathname));
        const restoreSession = async () => {
            try {
                const payload = await getAuthSession();
                if (!cancelled) await applyUserSession(payload);
            } catch (error) {
                if (cancelled) return;
                // 只有后端明确拒绝当前会话时才退出；网络与服务异常必须保留本地登录态。
                if (isExplicitlyUnauthenticated(error)) {
                    try {
                        await applyUserSession({ user: null, logicalModels: [] });
                    } catch (sessionError) {
                        if (!cancelled) setRecoveryError(sessionRecoveryErrorMessage(sessionError));
                    }
                    return;
                }
                setRecoveryError(sessionRecoveryErrorMessage(error));
            }
        };
        void restoreSession();
        return () => {
            cancelled = true;
        };
    }, [retrySequence]);

    if (recoveryError) {
        return (
            <main className="grid min-h-screen place-items-center bg-background p-6">
                <WorkspaceErrorState
                    title="登录服务暂时不可用"
                    description={`${recoveryError}。当前登录状态未被清除，请检查网络连接后重试。`}
                    actionLabel="重试"
                    onRetry={() => setRetrySequence((current) => current + 1)}
                />
            </main>
        );
    }

    return hydrated ? children : <FullScreenLoader />;
}
