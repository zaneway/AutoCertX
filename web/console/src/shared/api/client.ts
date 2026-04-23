export interface ApiErrorDetail {
  field?: string;
  reason?: string;
}

interface ErrorEnvelope {
  error?: {
    code?: string;
    message?: string;
    details?: ApiErrorDetail[];
  };
}

interface SessionBridge {
  getAccessToken: () => string | null;
  getScopeHeaders: () => Record<string, string>;
  handleUnauthorized: () => void;
}

interface RequestOptions {
  method?: string;
  body?: unknown;
  headers?: HeadersInit;
  auth?: boolean;
}

const defaultBridge: SessionBridge = {
  getAccessToken: () => null,
  getScopeHeaders: () => ({}),
  handleUnauthorized: () => undefined,
};

let sessionBridge: SessionBridge = defaultBridge;

const apiBaseUrl = (import.meta.env.VITE_API_BASE_URL ?? "").replace(/\/$/, "");

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly details: ApiErrorDetail[];

  constructor(status: number, code: string, message: string, details: ApiErrorDetail[] = []) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.details = details;
  }
}

export function registerApiSessionBridge(bridge: SessionBridge): void {
  sessionBridge = bridge;
}

function buildUrl(path: string): string {
  return path.startsWith("http://") || path.startsWith("https://") ? path : `${apiBaseUrl}${path}`;
}

async function parseResponseBody(response: Response): Promise<unknown> {
  const text = await response.text();
  if (!text) {
    return null;
  }

  try {
    return JSON.parse(text) as unknown;
  } catch {
    return text;
  }
}

export const apiClient = {
  async request<T>(path: string, options: RequestOptions = {}): Promise<T> {
    const { method = "GET", body, headers, auth = true } = options;
    const requestHeaders = new Headers(headers);

    requestHeaders.set("Accept", "application/json");

    if (body !== undefined && !(body instanceof FormData) && !requestHeaders.has("Content-Type")) {
      requestHeaders.set("Content-Type", "application/json");
    }

    if (auth) {
      const accessToken = sessionBridge.getAccessToken();
      if (accessToken) {
        requestHeaders.set("Authorization", `Bearer ${accessToken}`);
      }
      const scopeHeaders = sessionBridge.getScopeHeaders();
      Object.entries(scopeHeaders).forEach(([key, value]) => {
        requestHeaders.set(key, value);
      });
    }

    const response = await fetch(buildUrl(path), {
      method,
      headers: requestHeaders,
      body: body === undefined || body instanceof FormData ? (body as BodyInit | undefined) : JSON.stringify(body),
    });

    const payload = await parseResponseBody(response);
    if (!response.ok) {
      const envelope = (payload ?? {}) as ErrorEnvelope;
      const error = new ApiError(
        response.status,
        envelope.error?.code ?? "http_error",
        envelope.error?.message ?? response.statusText,
        envelope.error?.details ?? [],
      );
      if (auth && response.status === 401) {
        sessionBridge.handleUnauthorized();
      }
      throw error;
    }

    return payload as T;
  },
};
