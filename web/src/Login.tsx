import { FormEvent, useState } from "react";

const TOKEN_KEY = "cost-board:token";

export function getToken() {
  return localStorage.getItem(TOKEN_KEY);
}

export function setToken(token: string) {
  localStorage.setItem(TOKEN_KEY, token);
}

export function clearToken() {
  localStorage.removeItem(TOKEN_KEY);
}

export function Login({ onSuccess, onCancel }: { onSuccess: () => void; onCancel?: () => void }) {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);
    setLoading(true);

    try {
      const res = await fetch("/api/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username, password }),
      });

      if (res.status === 429) {
        setError("尝试过于频繁，请稍后再试。");
        return;
      }

      if (!res.ok) {
        setError("用户名或密码错误。");
        return;
      }

      const data = (await res.json()) as { token: string };
      setToken(data.token);
      onSuccess();
    } catch {
      setError("无法连接服务器，请检查网络后重试。");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="login-shell">
      <form className="login-card" onSubmit={handleSubmit}>
        <div className="login-icon">
          <svg viewBox="0 0 24 24" width="40" height="40" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
            <rect x="3" y="11" width="18" height="11" rx="2" ry="2"></rect>
            <path d="M7 11V7a5 5 0 0 1 10 0v4"></path>
          </svg>
        </div>
        <h1 className="login-title">Cost Board</h1>
        <p className="login-subtitle">登录以查看你的支出面板</p>

        <label className="login-field">
          <span className="field-label">用户名</span>
          <input
            autoComplete="username"
            autoFocus
            enterKeyHint="next"
            placeholder="用户名"
            required
            spellCheck={false}
            type="text"
            value={username}
            onChange={(event) => setUsername(event.target.value)}
          />
        </label>

        <label className="login-field">
          <span className="field-label">密码</span>
          <input
            autoComplete="current-password"
            enterKeyHint="go"
            placeholder="密码"
            required
            type="password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
          />
        </label>

        {error && <p className="login-error">{error}</p>}

        <button className="primary-button login-submit" disabled={loading} type="submit">
          {loading ? "登录中..." : "登录"}
        </button>
        {onCancel && (
          <button className="secondary-button login-cancel" type="button" onClick={onCancel}>
            返回
          </button>
        )}
      </form>
    </div>
  );
}
