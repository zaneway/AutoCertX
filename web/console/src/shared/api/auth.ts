import { apiClient } from "@/shared/api/client";

export interface NamedResource {
  id: string;
  name: string;
  code?: string;
  display_name?: string;
}

export interface AuthContextView {
  locale: string;
  available_locales: string[];
  user: NamedResource;
  tenant: NamedResource;
  project: NamedResource;
  environment: NamedResource;
  roles: string[];
}

export interface AuthLoginResponse {
  request_id: string;
  access_token: string;
  refresh_token: string;
  expires_in: number;
  context: AuthContextView;
}

export interface AuthMeResponse {
  request_id: string;
  data: AuthContextView;
}

interface LoginRequest {
  username: string;
  password: string;
}

interface RefreshRequest {
  refresh_token: string;
}

interface LocalePreferenceRequest {
  locale: string;
}

export function loginWithPassword(payload: LoginRequest): Promise<AuthLoginResponse> {
  return apiClient.request<AuthLoginResponse>("/api/v1/auth/login", {
    method: "POST",
    auth: false,
    body: payload,
  });
}

export function refreshSessionToken(refreshToken: string): Promise<AuthLoginResponse> {
  return apiClient.request<AuthLoginResponse>("/api/v1/auth/refresh", {
    method: "POST",
    auth: false,
    body: { refresh_token: refreshToken } satisfies RefreshRequest,
  });
}

export function logoutSession(): Promise<{ request_id: string; status: string }> {
  return apiClient.request<{ request_id: string; status: string }>("/api/v1/auth/logout", {
    method: "POST",
  });
}

export function fetchCurrentAuthContext(): Promise<AuthMeResponse> {
  return apiClient.request<AuthMeResponse>("/api/v1/auth/me");
}

export function updateLocalePreference(locale: string): Promise<AuthMeResponse> {
  return apiClient.request<AuthMeResponse>("/api/v1/auth/me/preferences", {
    method: "PATCH",
    body: { locale } satisfies LocalePreferenceRequest,
  });
}
