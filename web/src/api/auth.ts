const TOKEN_KEY = 'probex_token';

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY);
}

export function setToken(token: string) {
  localStorage.setItem(TOKEN_KEY, token);
}

export function clearToken() {
  localStorage.removeItem(TOKEN_KEY);
}

// login exchanges a password for a bearer token. Throws on wrong password.
export async function login(password: string): Promise<void> {
  const res = await fetch('/api/v1/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ password }),
  });
  const json = await res.json();
  if (!res.ok || json.error) throw new Error(json.error || 'login failed');
  setToken(json.data.token);
}

// authRequired asks the backend whether login is enabled on this instance.
export async function authRequired(): Promise<boolean> {
  try {
    const res = await fetch('/api/v1/mode');
    const json = await res.json();
    return !!json.data?.auth_required;
  } catch {
    return false;
  }
}
