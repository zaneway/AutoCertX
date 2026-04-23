<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";

import { useSessionStore } from "@/shared/auth/store";

const sessionStore = useSessionStore();
const { t } = useI18n();

const primaryRoute = computed(() => (sessionStore.isAuthenticated ? { name: "dashboard" } : { name: "login" }));
const primaryLabel = computed(() => (sessionStore.isAuthenticated ? t("common.backToDashboard") : t("common.backToLogin")));
</script>

<template>
  <section class="empty-state card">
    <div class="empty-state__body">
      <span class="pill pill--warn">404</span>
      <h1>{{ t("pages.notFound.title") }}</h1>
      <p>{{ t("pages.notFound.description") }}</p>
      <div class="empty-state__actions">
        <RouterLink class="button button--secondary" :to="primaryRoute">
          {{ primaryLabel }}
        </RouterLink>
      </div>
    </div>
  </section>
</template>
