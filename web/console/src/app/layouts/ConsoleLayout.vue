<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { RouterLink, RouterView, useRoute, useRouter } from "vue-router";
import { useI18n } from "vue-i18n";

import { useSessionStore } from "@/shared/auth/store";
import { type SupportedLocale } from "@/shared/i18n";
import { primaryNavItems } from "@/shared/navigation/catalog";
import { useShellSummaryQuery } from "@/shared/query/shell";
import { useLiveClock } from "@/shared/shell/useLiveClock";

const route = useRoute();
const router = useRouter();
const sessionStore = useSessionStore();
const { t } = useI18n();
const navigationOpen = ref(false);
const appVersion = __APP_VERSION__;

const summaryQuery = useShellSummaryQuery();
const { formatted: currentTime } = useLiveClock(() => sessionStore.locale);

const pageTitle = computed(() =>
  route.meta.titleKey ? t(String(route.meta.titleKey)) : t("app.brandTitle"),
);
const pageDescription = computed(() =>
  route.meta.descriptionKey ? t(String(route.meta.descriptionKey)) : t("shell.currentViewDescription"),
);
const sectionTitle = computed(() =>
  route.meta.sectionKey ? t(String(route.meta.sectionKey)) : t("sections.platform"),
);
const activeNavKey = computed(() => String(route.meta.navKey ?? ""));
const formattedContext = computed(() => {
  if (!sessionStore.context) {
    return t("common.unavailable");
  }
  return t("shell.contextFormat", {
    tenant: sessionStore.context.tenant.name,
    project: sessionStore.context.project.name,
    environment: sessionStore.context.environment.name,
  });
});
const formattedRoles = computed(() => {
  if (sessionStore.roles.length === 0) {
    return t("common.unavailable");
  }

  return sessionStore.roles
    .map((role) => {
      const key = `roles.${role}`;
      const translated = t(key);
      return translated === key ? role : translated;
    })
    .join(" / ");
});

const availableLocales = computed(() => sessionStore.availableLocales);
const counters = computed(() => summaryQuery.data.value ?? {});

function resolveBadge(counterKey?: string): string | null {
  if (!counterKey) {
    return null;
  }
  const value = counters.value[counterKey];
  return typeof value === "number" ? String(value) : null;
}

async function onLocaleChange(locale: SupportedLocale): Promise<void> {
  await sessionStore.changeLocale(locale);
}

async function onSignOut(): Promise<void> {
  await sessionStore.logout();
  await router.replace({ name: "login" });
}

function toggleNavigation(): void {
  navigationOpen.value = !navigationOpen.value;
}

watch(
  () => route.fullPath,
  () => {
    navigationOpen.value = false;
  },
);
</script>

<template>
  <div class="app-shell" :class="{ 'app-shell--nav-open': navigationOpen }">
    <button
      class="app-shell__scrim"
      type="button"
      :aria-label="t('common.closeMenu')"
      @click="navigationOpen = false"
    />

    <aside class="sidebar">
      <div class="sidebar__panel">
        <section class="brand">
          <span class="brand__eyebrow">{{ t("app.brandEyebrow") }}</span>
          <h1 class="brand__title">{{ t("app.brandTitle") }}</h1>
          <p class="brand__desc">{{ t("app.brandDescription") }}</p>
        </section>

        <div class="nav-section__label">{{ t("shell.navSection") }}</div>
        <nav class="nav-list" :aria-label="t('shell.menuLabel')">
          <RouterLink
            v-for="item in primaryNavItems"
            :key="item.key"
            :to="{ name: item.routeName }"
            class="nav-item"
            :class="{ 'nav-item--active': activeNavKey === item.key }"
          >
            <span class="nav-item__left">
              <span class="nav-item__icon" />
              <span class="nav-item__name">{{ t(item.labelKey) }}</span>
            </span>
            <span v-if="resolveBadge(item.counterKey)" class="nav-item__badge">
              {{ resolveBadge(item.counterKey) }}
            </span>
          </RouterLink>
        </nav>

        <section class="sidebar__summary">
          <h3>{{ t("shell.currentViewTitle") }}</h3>
          <p>{{ t("shell.currentViewDescription") }}</p>
        </section>
      </div>
    </aside>

    <div class="workspace">
      <header class="topbar">
        <div class="topbar__main">
          <button
            class="shell-toggle"
            type="button"
            :aria-label="navigationOpen ? t('common.closeMenu') : t('common.openMenu')"
            @click="toggleNavigation"
          >
            {{ t("shell.menuLabel") }}
          </button>

          <div class="topbar__context">
            <div class="breadcrumbs">
              <span>{{ sectionTitle }}</span>
              <span>{{ pageTitle }}</span>
            </div>
            <div class="topbar__heading">
              <h2>{{ pageTitle }}</h2>
              <p>{{ pageDescription }}</p>
            </div>
          </div>
        </div>

        <div class="topbar__meta">
          <div class="meta-chip">
            <span>{{ t("shell.versionLabel") }}</span>
            <strong class="mono">v{{ appVersion }}</strong>
          </div>
          <div class="meta-chip">
            <span>{{ t("shell.timeLabel") }}</span>
            <strong>{{ currentTime }}</strong>
          </div>
          <div class="meta-chip">
            <span>{{ t("shell.roleLabel") }}</span>
            <strong>{{ formattedRoles }}</strong>
          </div>
          <div class="meta-chip">
            <span>{{ t("shell.contextLabel") }}</span>
            <strong>{{ formattedContext }}</strong>
          </div>
          <div class="meta-chip meta-chip--locale">
            <span>{{ t("shell.languageLabel") }}</span>
            <div class="locale-switch">
              <button
                v-for="locale in availableLocales"
                :key="locale"
                type="button"
                class="locale-switch__btn"
                :class="{ 'locale-switch__btn--active': sessionStore.locale === locale }"
                @click="onLocaleChange(locale)"
              >
                {{ locale }}
              </button>
            </div>
          </div>
          <button class="button button--ghost button--compact" type="button" @click="onSignOut">
            {{ t("common.signOut") }}
          </button>
        </div>
      </header>

      <main class="page">
        <div class="page__status" v-if="summaryQuery.isFetching">
          {{ t("shell.riskCountersPending") }}
        </div>
        <RouterView />
      </main>
    </div>
  </div>
</template>
