export const REFRESH_RATE_MS = 2000; // 2 seconds
export const LOG_REFRESH_RATE_MS = 10000; // 10 seconds

const viteEnv = import.meta.env ?? {};

// The dashboard is served by the daemon itself, so same-origin is the default:
// the session cookie flows automatically, no host is hard-coded, and a reverse
// proxy just works. The env overrides remain for split deployments.
const origin = typeof window !== 'undefined' ? window.location.origin : 'http://localhost:8080';

export const API_BASE_URL = viteEnv.VITE_SYS_SENTIENT_API_URL || `${origin}/api`;
export const WS_BASE_URL =
  viteEnv.VITE_SYS_SENTIENT_WS_URL || `${origin.replace(/^http/, 'ws')}/ws/metrics`;

/**
 * No credentials in the URL. The previous build appended the API key as
 * `?api_key=`, which put it in every proxy and browser-history log; the
 * browser now attaches the session cookie itself on a same-origin upgrade.
 */
export const metricsWebSocketURL = (): string => WS_BASE_URL;
