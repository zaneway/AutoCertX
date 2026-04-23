<script setup lang="ts">
import { useI18n } from "vue-i18n";

const { t } = useI18n();
</script>

<template>
  <section class="empty-state card">
    <div class="empty-state__body">
      <span class="pill pill--danger">403</span>
      <h1>{{ t("pages.forbidden.title") }}</h1>
      <p>{{ t("pages.forbidden.description") }}</p>
      <div class="empty-state__actions">
        <RouterLink class="button button--secondary" :to="{ name: 'dashboard' }">
          {{ t("common.backToDashboard") }}
        </RouterLink>
      </div>
    </div>
  </section>
</template>
