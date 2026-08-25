import { type FormEvent, useEffect, useState, type ReactNode } from "react";
import { App } from "antd";
import { Info, TriangleAlert } from "lucide-react";
import { useNavigate, useSearchParams } from "react-router";

import { applyUserSession } from "@/lib/user-session";
import { getAuthSession, getAuthSettings, linuxDOLoginURL, register, sendRegistrationEmailCode } from "@/services/api/auth";
import { LinuxDOIcon } from "./auth-scene";

type AuthSettings = Awaited<ReturnType<typeof getAuthSettings>>;

export default function RegisterPage() {
    const navigate = useNavigate();
    const [params] = useSearchParams();
    const { message } = App.useApp();
    const [settings, setSettings] = useState<AuthSettings | null>(null);
    const [username, setUsername] = useState("");
    const [email, setEmail] = useState("");
    const [emailCode, setEmailCode] = useState("");
    const [displayName, setDisplayName] = useState("");
    const [password, setPassword] = useState("");
    const [confirmPassword, setConfirmPassword] = useState("");
    const [showPasswords, setShowPasswords] = useState(false);
    const [submitting, setSubmitting] = useState(false);
    const [sendingCode, setSendingCode] = useState(false);
    const [countdown, setCountdown] = useState(0);
    const next = safeNext(params.get("next"));

    useEffect(() => {
        let cancelled = false;
        void getAuthSettings()
            .then((value) => !cancelled && setSettings(value))
            .catch((error) => !cancelled && message.error(error instanceof Error ? error.message : "读取注册设置失败"));
        return () => { cancelled = true; };
    }, [message]);

    useEffect(() => {
        if (countdown <= 0) return;
        const timer = window.setInterval(() => setCountdown((value) => Math.max(0, value - 1)), 1000);
        return () => window.clearInterval(timer);
    }, [countdown]);

    const sendCode = async () => {
        if (!email.trim()) {
            message.warning("请先输入邮箱");
            return;
        }
        setSendingCode(true);
        try {
            await sendRegistrationEmailCode(email.trim());
            setCountdown(60);
            message.success("验证码已发送，请检查邮箱");
        } catch (error) {
            message.error(error instanceof Error ? error.message : "发送验证码失败");
        } finally {
            setSendingCode(false);
        }
    };

    const submit = async (event: FormEvent<HTMLFormElement>) => {
        event.preventDefault();
        if (password !== confirmPassword) {
            message.error("两次输入的密码不一致");
            return;
        }
        setSubmitting(true);
        try {
            await register({ username, email, emailCode, displayName, password });
            await applyUserSession(await getAuthSession());
            if (!settings?.firstUser) window.sessionStorage.setItem("infinite-canvas:model-setup-guide", "1");
            message.success(settings?.firstUser ? "管理员账号已创建" : "注册成功");
            navigate(next, { replace: true });
        } catch (error) {
            message.error(error instanceof Error ? error.message : "注册失败");
        } finally {
            setSubmitting(false);
        }
    };

    const registrationClosed = settings?.registrationEnabled === false;
    const mailUnavailable = Boolean(settings && !settings.firstUser && settings.emailCodeRequired && !settings.emailEnabled);
    const disabled = registrationClosed || mailUnavailable;
    const requireCode = Boolean(settings && !settings.firstUser && settings.emailCodeRequired);

    return (
        <form className="auth-form auth-form-register" onSubmit={submit} aria-busy={submitting}>
            {settings?.firstUser ? <Notice icon={<Info size={15} />}>首个账号自动成为管理员，邮箱验证码暂不要求。</Notice> : null}
            {registrationClosed ? <Notice icon={<TriangleAlert size={15} />} warning>当前已关闭普通注册，请联系管理员创建账号。</Notice> : null}
            {mailUnavailable ? <Notice icon={<TriangleAlert size={15} />} warning>管理员尚未配置注册邮件，普通邮箱注册暂不可用。</Notice> : null}

            <AuthField label="用户名" inputId="auth-register-username">
                <input id="auth-register-username" name="username" type="text" value={username} onChange={(event) => setUsername(event.target.value)} placeholder="3-32 位字符" autoComplete="username" required disabled={disabled} />
            </AuthField>
            <AuthField label="显示名称" inputId="auth-register-display-name">
                <input id="auth-register-display-name" name="displayName" type="text" value={displayName} onChange={(event) => setDisplayName(event.target.value)} placeholder="不填则使用用户名" disabled={disabled} />
            </AuthField>

            <AuthField label="邮箱" inputId="auth-register-email" wide>
                <input id="auth-register-email" name="email" type="email" value={email} onChange={(event) => setEmail(event.target.value)} placeholder="用于登录与安全验证" autoComplete="email" required={!settings?.firstUser} disabled={disabled} />
            </AuthField>

            {requireCode ? (
                <AuthField label="邮箱验证码" inputId="auth-register-email-code" wide>
                    <span className="auth-code-row">
                        <input id="auth-register-email-code" name="emailCode" type="text" value={emailCode} onChange={(event) => setEmailCode(event.target.value.replace(/\D/g, "").slice(0, 6))} placeholder="6 位验证码" inputMode="numeric" autoComplete="one-time-code" required disabled={disabled} />
                        <button className="button button-outline" type="button" disabled={disabled || countdown > 0 || sendingCode} onClick={() => void sendCode()}>
                            {sendingCode ? "发送中" : countdown > 0 ? `${countdown}s` : "获取验证码"}
                        </button>
                    </span>
                </AuthField>
            ) : null}

            <AuthField label="密码" inputId="auth-register-password">
                <span className="password-field">
                    <input id="auth-register-password" name="password" type={showPasswords ? "text" : "password"} value={password} onChange={(event) => setPassword(event.target.value)} placeholder="至少 8 位" autoComplete="new-password" required disabled={disabled} />
                    <button type="button" aria-label={showPasswords ? "隐藏密码" : "显示密码"} onClick={() => setShowPasswords((value) => !value)}>{showPasswords ? "隐藏" : "显示"}</button>
                </span>
            </AuthField>
            <AuthField label="确认密码" inputId="auth-register-confirm-password">
                <span className="password-field">
                    <input id="auth-register-confirm-password" name="confirmPassword" type={showPasswords ? "text" : "password"} value={confirmPassword} onChange={(event) => setConfirmPassword(event.target.value)} placeholder="再次输入密码" autoComplete="new-password" required disabled={disabled} />
                </span>
            </AuthField>

            <button className="button button-light auth-submit" type="submit" disabled={disabled || submitting}>
                <span className={submitting ? "auth-loading-label" : undefined}>{submitting ? "创建中" : "创建账号"}</span>
            </button>
            {settings?.linuxdoEnabled ? (
                <a className="button button-outline auth-provider" href={linuxDOLoginURL(next)}>
                    <LinuxDOIcon />
                    使用 Linux.do 注册 / 登录
                </a>
            ) : <span aria-hidden="true" />}
            <p className="form-status" role="status" />
        </form>
    );
}

function AuthField({ label, inputId, wide = false, children }: { label: string; inputId: string; wide?: boolean; children: ReactNode }) {
    return <div className={`auth-field${wide ? " auth-field-wide" : ""}`}><label htmlFor={inputId}>{label}</label>{children}</div>;
}

function Notice({ icon, warning = false, children }: { icon: ReactNode; warning?: boolean; children: ReactNode }) {
    return <div className={`auth-notice${warning ? " is-warning" : ""}`}><span aria-hidden="true">{icon}</span><span>{children}</span></div>;
}

function safeNext(value: string | null) {
    if (!value || !value.startsWith("/") || value.startsWith("//")) return "/create";
    return value;
}
