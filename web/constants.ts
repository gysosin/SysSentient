export const APP_NAME = "SysSentient";
export const REFRESH_RATE_MS = 2000; // 2 seconds
export const HISTORY_LENGTH = 30; // 60 seconds of history on chart

export const API_BASE_URL = import.meta.env.VITE_SYS_SENTIENT_API_URL || 'http://localhost:8080/api';
export const WS_BASE_URL = import.meta.env.VITE_SYS_SENTIENT_WS_URL || 'ws://localhost:8080/ws/metrics';
export const API_KEY = import.meta.env.VITE_SYS_SENTIENT_API_KEY || '';

export const authHeaders = (): HeadersInit => {
  if (!API_KEY) return {};
  return { 'X-API-Key': API_KEY };
};

export const metricsWebSocketURL = (): string => {
  if (!API_KEY) return WS_BASE_URL;

  const url = new URL(WS_BASE_URL);
  url.searchParams.set('api_key', API_KEY);
  return url.toString();
};

// Gemini Model
export const GEMINI_MODEL = 'gemini-2.5-flash-lite';

// PII Scrubbing Patterns
export const PII_PATTERNS = {
  IPV4: /\b(?:\d{1,3}\.){3}\d{1,3}\b/g,
  IPV6: /([0-9a-fA-F]{1,4}:){7,7}[0-9a-fA-F]{1,4}|([0-9a-fA-F]{1,4}:){1,7}:|([0-9a-fA-F]{1,4}:){1,6}:[0-9a-fA-F]{1,4}|([0-9a-fA-F]{1,4}:){1,5}(:[0-9a-fA-F]{1,4}){1,2}|([0-9a-fA-F]{1,4}:){1,4}(:[0-9a-fA-F]{1,4}){1,3}|([0-9a-fA-F]{1,4}:){1,3}(:[0-9a-fA-F]{1,4}){1,4}|([0-9a-fA-F]{1,4}:){1,2}(:[0-9a-fA-F]{1,4}){1,5}|[0-9a-fA-F]{1,4}:((:[0-9a-fA-F]{1,4}){1,6})|:((:[0-9a-fA-F]{1,4}){1,7}|:)|fe80:(:[0-9a-fA-F]{0,4}){0,4}%[0-9a-zA-Z]{1,}|::(ffff(:0{1,4}){0,1}:){0,1}((25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9])\.){3,3}(25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9])|([0-9a-fA-F]{1,4}:){1,4}:((25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9])\.){3,3}(25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9])/g,
  EMAIL: /[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}/g,
  // Simple heuristic for home directories to mask usernames
  HOME_DIR: /\/home\/([a-zA-Z0-9_-]+)\//g
};
