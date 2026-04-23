import { defineStore } from "pinia";

import {
  fetchCurrentAuthContext,
  loginWithPassword,
  logoutSession,
  refreshSessionToken,
  updateLocalePreference,
  type AuthContextView,
  type AuthLoginResponse,
} from "@/shared/api/auth";
import { registerApiSessionBridge } from "@/shared/api/client";
import { DEFAULT_LOCALE, SUPPORTED_LOCALES, i18n, resolveSupportedLocale, resolveSupportedLocales, type SupportedLocale } from "@/shared/i18n";

const storageKey = "autocertx.console.session";

interface SessionSnapshot {
  accessToken: string | null;
  refreshToken: string | null;
  expiresAt: number | null;
  context: AuthContextView | null;
  explicitLocale: SupportedLocale | null;
}

interface SessionState extends SessionSnapshot {
  availableLocales: SupportedLocale[];
  bootstrapped: boolean;
}

const emptySnapshot: SessionSnapshot = {
  accessToken: null,
  refreshToken: null,
  expiresAt: null,
  context: null,
  explicitLocale: null,
};

function canUseStorage(): boolean {
  return typeof window !== "undefined" && typeof window.localStorage !== "undefined";
}

function readSnapshot(): SessionSnapshot {
  if (!canUseStorage()) {
    return { ...emptySnapshot };
  }

  const raw = window.localStorage.getItem(storageKey);
  if (!raw) {
    return { ...emptySnapshot };
  }

  try {
    const parsed = JSON.parse(raw) as Partial<SessionSnapshot>;
    return {
      accessToken: typeof parsed.accessToken === "string" ? parsed.accessToken : null,
      refreshToken: typeof parsed.refreshToken === "string" ? parsed.refreshToken : null,
      expiresAt: typeof parsed.expiresAt === "number" ? parsed.expiresAt : null,
      context: parsed.context ?? null,
      explicitLocale: parsed.explicitLocale ? resolveSupportedLocale(parsed.explicitLocale) : null,
    };
  } catch {
    return { ...emptySnapshot };
  }
}

function writeSnapshot(snapshot: SessionSnapshot): void {
  if (!canUseStorage()) {
    return;
  }

  window.localStorage.setItem(storageKey, JSON.stringify(snapshot));
}

export const useSessionStore = defineStore("session", {
  state: (): SessionState => ({
    ...emptySnapshot,
    availableLocales: [...SUPPORTED_LOCALES],
    bootstrapped: false,
  }),

  getters: {
    isAuthenticated: (state): boolean => Boolean(state.accessToken && state.context),
    locale(state): SupportedLocale {
      return resolveSupportedLocale(state.context?.locale ?? state.explicitLocale ?? DEFAULT_LOCALE);
    },
    roles(state): string[] {
      return state.context?.roles ?? [];
    },
    scopeHeaders(state): Record<string, string> {
      if (!state.context) {
        return {};
      }

      return {
        "X-Tenant-Id": state.context.tenant.id,
        "X-Project-Id": state.context.project.id,
        "X-Environment-Id": state.context.environment.id,
      };
    },
  },

  actions: {
    bindApiClient(): void {
      registerApiSessionBridge({
        getAccessToken: () => this.accessToken,
        getScopeHeaders: () => this.scopeHeaders,
        handleUnauthorized: () => {
          const locale = this.locale;
          this.clearSession(locale);
        },
      });
    },

    syncLocale(locale: SupportedLocale): void {
      i18n.global.locale.value = locale;
    },

    persist(): void {
      writeSnapshot({
        accessToken: this.accessToken,
        refreshToken: this.refreshToken,
        expiresAt: this.expiresAt,
        context: this.context,
        explicitLocale: this.explicitLocale,
      });
    },

    applySnapshot(snapshot: SessionSnapshot): void {
      this.accessToken = snapshot.accessToken;
      this.refreshToken = snapshot.refreshToken;
      this.expiresAt = snapshot.expiresAt;
      this.context = snapshot.context;
      this.explicitLocale = snapshot.explicitLocale;
      this.availableLocales = resolveSupportedLocales(snapshot.context?.available_locales);
      this.syncLocale(this.locale);
    },

    applyAuthResponse(response: AuthLoginResponse): void {
      this.accessToken = response.access_token;
      this.refreshToken = response.refresh_token;
      this.expiresAt = Date.now() + response.expires_in * 1000;
      this.context = response.context;
      this.explicitLocale = resolveSupportedLocale(response.context.locale);
      this.availableLocales = resolveSupportedLocales(response.context.available_locales);
      this.syncLocale(this.locale);
      this.persist();
    },

    needsRefresh(): boolean {
      if (!this.accessToken || !this.expiresAt) {
        return Boolean(this.refreshToken);
      }

      return Date.now() >= this.expiresAt - 30_000;
    },

    async bootstrap(): Promise<void> {
      if (this.bootstrapped) {
        this.bindApiClient();
        return;
      }

      this.applySnapshot(readSnapshot());
      this.bindApiClient();
      this.bootstrapped = true;

      if (!this.accessToken && !this.refreshToken) {
        return;
      }

      if (this.needsRefresh()) {
        await this.refresh();
        return;
      }

      if (this.accessToken && !this.context) {
        try {
          await this.fetchContext();
        } catch {
          await this.refresh();
        }
      }
    },

    async login(username: string, password: string): Promise<void> {
      const response = await loginWithPassword({ username, password });
      this.applyAuthResponse(response);
    },

    async refresh(): Promise<boolean> {
      if (!this.refreshToken) {
        this.clearSession(this.locale);
        return false;
      }

      try {
        const response = await refreshSessionToken(this.refreshToken);
        this.applyAuthResponse(response);
        return true;
      } catch {
        this.clearSession(this.locale);
        return false;
      }
    },

    async fetchContext(): Promise<void> {
      const response = await fetchCurrentAuthContext();
      this.context = response.data;
      this.explicitLocale = resolveSupportedLocale(response.data.locale);
      this.availableLocales = resolveSupportedLocales(response.data.available_locales);
      this.syncLocale(this.locale);
      this.persist();
    },

    async ensureAuthenticated(): Promise<boolean> {
      if (!this.bootstrapped) {
        await this.bootstrap();
      }

      if (!this.accessToken && this.refreshToken) {
        return this.refresh();
      }
      if (!this.accessToken) {
        return false;
      }
      if (this.needsRefresh()) {
        return this.refresh();
      }
      if (!this.context) {
        try {
          await this.fetchContext();
        } catch {
          return this.refresh();
        }
      }

      return Boolean(this.accessToken && this.context);
    },

    async logout(): Promise<void> {
      const locale = this.locale;
      try {
        if (this.accessToken) {
          await logoutSession();
        }
      } catch {
        // Logging out should still clear local state even if the session endpoint fails.
      }
      this.clearSession(locale);
    },

    async changeLocale(locale: SupportedLocale): Promise<void> {
      if (locale === this.locale) {
        return;
      }

      if (!this.isAuthenticated) {
        this.explicitLocale = locale;
        this.syncLocale(locale);
        this.persist();
        return;
      }

      const response = await updateLocalePreference(locale);
      this.context = response.data;
      this.explicitLocale = resolveSupportedLocale(response.data.locale);
      this.availableLocales = resolveSupportedLocales(response.data.available_locales);
      this.syncLocale(this.locale);
      this.persist();
    },

    clearSession(preserveLocale?: SupportedLocale): void {
      this.accessToken = null;
      this.refreshToken = null;
      this.expiresAt = null;
      this.context = null;
      this.explicitLocale = preserveLocale ?? this.explicitLocale ?? DEFAULT_LOCALE;
      this.availableLocales = [...SUPPORTED_LOCALES];
      this.syncLocale(this.locale);
      this.persist();
    },
  },
});
