/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_SYS_SENTIENT_API_URL?: string;
  readonly VITE_SYS_SENTIENT_WS_URL?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
