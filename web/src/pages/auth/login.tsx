import { type FormEvent, useEffect, useState } from "react";
import { App } from "antd";
import { ArrowLeft, ArrowRight, TriangleAlert } from "lucide-react";
import { useNavigate, useSearchParams } from "react-router";

import { applyUserSession } from "@/lib/user-session";
import { getAuthSession, getAuthSettings, linuxDOLoginURL, login, requestPasswordResetCode, resetPassword } from "@/services/api/auth";
import { LinuxDOIcon } from "./auth-scene";

type LoginMode = "login" | "request-reset" | "confirm-reset";

export default function LoginPage() {
    const navigate = useNavigate();
    const [params] = useSearchParams();
    const { message } = App.useApp();
    const [username, setUsername] = useState("");
    const [password, setPassword] = useState("");
    const [showPassword, setShowPassword] = useState(false);
    const [submitting, setSubmitting] = useState(false);
    const [linuxdoEnabled, setLinuxdoEnabled] = useState(false);
    const [passwordResetEnabled, setPasswordResetEnabled] = useState<boolean | null>(null);
    const [mode, setMode] = useState<LoginMode>("login");
    const [resetEmail, setResetEmail] = useState("");
    const [emailCode, setEmailCode] = useState("");
    const [newPassword, setNewPassword] = useState("");
    const [confirmPassword, setConfirmPassword] = useState("");
    const [showResetPasswords, setShowResetPasswords] = useState(false);
    const [sendingCode, setSendingCode] = useState(false);
    const [resetting, setResetting] = useState(false);
    const [countdown, setCountdown] = useState(0);
    const next = safeNext(params.get("next"));

    useEffect(() => {
        void getAuthSettings()
            .then((settings) => {
                setLinuxdoEnabled(settings.linuxdoEnabled);
                setPasswordResetEnabled(settings.passwordResetEnabled);
            })
            .catch(() => undefined);
        const oauthError = params.get("oauth_error");
        if (oauthError) message.error(oauthError);
    }, [message, params]);

    useEffect(() => {
        if (countdown <= 0) return;
        const timer = window.setInterval(() => setCountdown((value) => Math.max(0, value - 1)), 1000);
        return () => window.clearInterval(timer);
    }, [countdown]);

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

    const beginPasswordReset = () => {
        const identity = username.trim();
        if (identity.includes("@")) setResetEmail(identity);
        setMode("request-reset");
    };

    const returnToLogin = () => {
        setMode("login");
        setEmailCode("");
        setNewPassword("");
        setConfirmPassword("");
        setShowResetPasswords(false);
    };

    const sendResetCode = async (event?: FormEvent<HTMLFormElement>) => {
        event?.preventDefault();
        const email = resetEmail.trim();
        if (!email) {
            message.warning("请先输入注册邮箱");
            return;
        }
        if (passwordResetEnabled === false) {
            message.error("邮件服务尚未配置，请联系管理员重置密码");
            return;
        }
        setSendingCode(true);
        try {
            await requestPasswordResetCode(email);
            setResetEmail(email);
            setCountdown(60);
            setMode("confirm-reset");
            message.success("如果该邮箱已注册，验证码将发送至邮箱");
        } catch (error) {
            message.error(error instanceof Error ? error.message : "暂时无法发送验证码，请联系管理员");
        } finally {
            setSendingCode(false);
        }
    };

    const confirmReset = async (event: FormEvent<HTMLFormElement>) => {
        event.preventDefault();
        if (newPassword !== confirmPassword) {
            message.error("两次输入的密码不一致");
            return;
        }
        setResetting(true);
        try {
            await resetPassword({ email: resetEmail, emailCode, newPassword });
            setUsername(resetEmail);
            setPassword("");
            returnToLogin();
            message.success("密码已重置，请使用新密码登录");
        } catch (error) {
            message.error(error instanceof Error ? error.message : "密码重置失败");
        } finally {
            setResetting(false);
        }
    };


    if (mode !== "login") {
        const unavailable = passwordResetEnabled === false;
        const confirming = mode === "confirm-reset";
        return (
            <form className="auth-form auth-form-reset" onSubmit={confirming ? confirmReset : sendResetCode} aria-busy={sendingCode || resetting}>
                <div className="auth-reset-header">
                    <button className="auth-back-action" type="button" onClick={returnToLogin}>
                        <ArrowLeft aria-hidden="true" />
                        返回登录
                    </button>
                    <div>
                        <strong>{confirming ? "设置新密码" : "找回密码"}</strong>
                        <small>{confirming ? `验证码已发送至 ${resetEmail}` : "使用注册邮箱验证账号归属"}</small>
                    </div>
                </div>

                {unavailable ? (
                    <div className="auth-notice is-warning" role="status">
                        <TriangleAlert size={15} aria-hidden="true" />
                        邮件服务尚未配置，暂时无法在线找回密码，请联系管理员处理。
                    </div>
                ) : null}

                {!confirming ? (
                    <div className="auth-field">
                        <label htmlFor="auth-reset-email">注册邮箱</label>
                        <input id="auth-reset-email" name="email" type="email" value={resetEmail} onChange={(event) => setResetEmail(event.target.value)} placeholder="输入注册时使用的邮箱" autoComplete="email" autoFocus required disabled={unavailable || sendingCode} />
                    </div>
                ) : (
                    <>
                        <div className="auth-field">
                            <label htmlFor="auth-reset-code">邮箱验证码</label>
                            <span className="auth-code-row">
                                <input id="auth-reset-code" name="emailCode" type="text" value={emailCode} onChange={(event) => setEmailCode(event.target.value.replace(/\D/g, "").slice(0, 6))} placeholder="6 位验证码" inputMode="numeric" autoComplete="one-time-code" autoFocus required disabled={resetting} />
                                <button className="button button-outline" type="button" disabled={countdown > 0 || sendingCode || resetting} onClick={() => void sendResetCode()}>
                                    {sendingCode ? "发送中" : countdown > 0 ? `${countdown}s` : "重新发送"}
                                </button>
                            </span>
                        </div>
                        <div className="auth-reset-passwords">
                            <div className="auth-field">
                                <label htmlFor="auth-reset-password">新密码</label>
                                <span className="password-field">
                                    <input id="auth-reset-password" name="newPassword" type={showResetPasswords ? "text" : "password"} value={newPassword} onChange={(event) => setNewPassword(event.target.value)} placeholder="至少 8 位" autoComplete="new-password" minLength={8} required disabled={resetting} />
                                    <button type="button" aria-label={showResetPasswords ? "隐藏密码" : "显示密码"} onClick={() => setShowResetPasswords((value) => !value)}>
                                        {showResetPasswords ? "隐藏" : "显示"}
                                    </button>
                                </span>
                            </div>
                            <div className="auth-field">
                                <label htmlFor="auth-reset-confirm-password">确认密码</label>
                                <span className="password-field">
                                    <input id="auth-reset-confirm-password" name="confirmPassword" type={showResetPasswords ? "text" : "password"} value={confirmPassword} onChange={(event) => setConfirmPassword(event.target.value)} placeholder="再次输入新密码" autoComplete="new-password" minLength={8} required disabled={resetting} />
                                    <button type="button" aria-label={showResetPasswords ? "隐藏密码" : "显示密码"} onClick={() => setShowResetPasswords((value) => !value)}>
                                        {showResetPasswords ? "隐藏" : "显示"}
                                    </button>
                                </span>
                            </div>
                        </div>
                    </>
                )}

                <button className="button button-light auth-submit" type="submit" disabled={unavailable || sendingCode || resetting}>
                    <span className="auth-submit-copy">
                        <strong>{sendingCode ? "正在发送" : resetting ? "正在重置" : confirming ? "确认重置密码" : "发送验证码"}</strong>
                        <small>{confirming ? "完成后返回登录" : "验证码 10 分钟内有效"}</small>
                    </span>
                    {sendingCode || resetting ? <span className="auth-submit-spinner" aria-hidden="true" /> : <ArrowRight aria-hidden="true" />}
                </button>
            </form>
        );
    }

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
                <button className="auth-forgot-action" type="button" onClick={beginPasswordReset}>忘记密码？</button>
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
