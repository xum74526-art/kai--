import { type FormEvent, useEffect, useState } from "react";
import { App } from "antd";
import { ArrowRight } from "lucide-react";
import { useNavigate, useSearchParams } from "react-router";

import { applyUserSession } from "@/lib/user-session";
import { getAuthSession, getAuthSettings, linuxDOLoginURL, login } from "@/services/api/auth";
import { LinuxDOIcon } from "./auth-scene";

export default function LoginPage() {
    const navigate = useNavigate();
    const [params] = useSearchParams();
    const { message } = App.useApp();
    const [username, setUsername] = useState("");
    const [password, setPassword] = useState("");
    const [showPassword, setShowPassword] = useState(false);
    const [submitting, setSubmitting] = useState(false);
    const [linuxdoEnabled, setLinuxdoEnabled] = useState(false);
    const next = safeNext(params.get("next"));

    useEffect(() => {
        void getAuthSettings().then((settings) => setLinuxdoEnabled(settings.linuxdoEnabled)).catch(() => undefined);
        const oauthError = params.get("oauth_error");
        if (oauthError) message.error(oauthError);
    }, [message, params]);

    const submit = async (event: FormEvent<HTMLFormElement>) => {
        event.preventDefault();
        setSubmitting(true);
        try {
            await login({ username, password });
            await applyUserSession(await getAuthSession());
            message.success("登录成功");
            navigate(next, { replace: true });
        } catch (error) {
            message.error(error instanceof Error ? error.message : "登录失败");
        } finally {
            setSubmitting(false);
        }
    };

    return (
        <form className="auth-form auth-form-login" onSubmit={submit} aria-busy={submitting}>
            <div className="auth-field">
                <label htmlFor="auth-identity">用户名或邮箱</label>
                <input
                    id="auth-identity"
                    name="username"
                    type="text"
                    value={username}
                    onChange={(event) => setUsername(event.target.value)}
                    placeholder="输入用户名或邮箱"
                    autoComplete="username"
                    required
                />
            </div>
            <div className="auth-field">
                <label htmlFor="auth-password">密码</label>
                <span className="password-field">
                    <input
                        id="auth-password"
                        name="password"
                        type={showPassword ? "text" : "password"}
                        value={password}
                        onChange={(event) => setPassword(event.target.value)}
                        placeholder="输入密码"
                        autoComplete="current-password"
                        required
                    />
                    <button
                        type="button"
                        aria-label={showPassword ? "隐藏密码" : "显示密码"}
                        onClick={() => setShowPassword((value) => !value)}
                    >
                        {showPassword ? "隐藏" : "显示"}
                    </button>
                </span>
            </div>
            <button className="button button-light auth-submit" type="submit" disabled={submitting}>
                <span className="auth-submit-copy">
                    <strong>{submitting ? "正在登录" : "登录并继续"}</strong>
                    <small>{submitting ? "正在恢复你的工作区" : "进入 KAI影创作台"}</small>
                </span>
                {submitting ? <span className="auth-submit-spinner" aria-hidden="true" /> : <ArrowRight aria-hidden="true" />}
            </button>
            {linuxdoEnabled ? (
                <a className="button button-outline auth-provider" href={linuxDOLoginURL(next)}>
                    <LinuxDOIcon />
                    使用 Linux.do 登录
                </a>
            ) : null}
        </form>
    );
}

function safeNext(value: string | null) {
    if (!value || !value.startsWith("/") || value.startsWith("//")) return "/create";
    return value;
}
