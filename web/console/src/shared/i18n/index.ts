import { createI18n } from "vue-i18n";

import { messages } from "@/shared/i18n/messages";

export const SUPPORTED_LOCALES = ["zh-CN", "en-US"] as const;
export type SupportedLocale = (typeof SUPPORTED_LOCALES)[number];
export const DEFAULT_LOCALE: SupportedLocale = "zh-CN";

export function resolveSupportedLocale(locale?: string | null): SupportedLocale {
  return locale === "en-US" ? "en-US" : "zh-CN";
}

export function resolveSupportedLocales(locales?: readonly string[] | null): SupportedLocale[] {
  if (!locales || locales.length === 0) {
    return [...SUPPORTED_LOCALES];
  }

  const normalized = locales
    .map((locale) => resolveSupportedLocale(locale))
    .filter((locale, index, values) => values.indexOf(locale) === index);

  return normalized.length > 0 ? normalized : [...SUPPORTED_LOCALES];
}

export const i18n = createI18n({
  legacy: false,
  locale: DEFAULT_LOCALE,
  fallbackLocale: DEFAULT_LOCALE,
  messages,
});
