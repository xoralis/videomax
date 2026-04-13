// services/authService.js

const TOKEN_KEY = 'videomax_token';
const USER_KEY = 'videomax_user';

// ─── Token 存储 ────────────────────────────────
export function getToken() {
  return localStorage.getItem(TOKEN_KEY);
}

export function setToken(token) {
  localStorage.setItem(TOKEN_KEY, token);
}

export function clearAuth() {
  localStorage.removeItem(TOKEN_KEY);
  localStorage.removeItem(USER_KEY);
}

export function getUser() {
  try {
    return JSON.parse(localStorage.getItem(USER_KEY));
  } catch {
    return null;
  }
}

function setUser(user) {
  localStorage.setItem(USER_KEY, JSON.stringify(user));
}

export function isAuthenticated() {
  return !!getToken();
}

// ─── 带 Auth Header 的 fetch 封装 ──────────────
export async function authFetch(url, options = {}) {
  const token = getToken();
  const headers = {
    'Content-Type': 'application/json',
    ...(options.headers || {}),
  };
  if (token) {
    headers['Authorization'] = `Bearer ${token}`;
  }
  // multipart 时不强制 Content-Type，让浏览器自动设置 boundary
  if (options.body instanceof FormData) {
    delete headers['Content-Type'];
  }
  return fetch(url, { ...options, headers });
}

// ─── 注册 ──────────────────────────────────────
export async function register(username, email, password) {
  const res = await fetch('/api/auth/register', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, email, password }),
  });
  const data = await res.json();
  if (data.code !== 0) throw new Error(data.msg || '注册失败');
  setToken(data.token);
  setUser({ user_id: data.user_id, username: data.username, email: data.email });
  return data;
}

// ─── 登录 ──────────────────────────────────────
export async function login(email, password) {
  const res = await fetch('/api/auth/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email, password }),
  });
  const data = await res.json();
  if (data.code !== 0) throw new Error(data.msg || '登录失败');
  setToken(data.token);
  setUser({ user_id: data.user_id, username: data.username, email: data.email });
  return data;
}

// ─── 退出 ──────────────────────────────────────
export function logout() {
  clearAuth();
}
