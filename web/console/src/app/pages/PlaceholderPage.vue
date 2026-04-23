<script setup lang="ts">
import { computed } from "vue";
import { useRoute } from "vue-router";
import { useI18n } from "vue-i18n";

import PermissionGuard from "@/shared/components/PermissionGuard.vue";

const route = useRoute();
const { t } = useI18n();

const milestoneLabel = computed(() => t(`milestones.${String(route.meta.milestone ?? "T13")}`));
const permissionLabel = computed(() => String(route.meta.permission ?? t("common.protectedRoute")));
</script>

<template>
  <section class="page-grid">
    <article class="card placeholder-card placeholder-card--hero">
      <div class="placeholder-card__meta">
        <span class="pill pill--brand">{{ t("placeholder.phaseTitle") }}</span>
        <span class="pill">{{ milestoneLabel }}</span>
      </div>
      <p>{{ t("placeholder.phaseDescription") }}</p>
      <p class="placeholder-card__foot mono">{{ permissionLabel }}</p>
    </article>

    <article class="card placeholder-card">
      <div class="card__header card__header--tight">
        <div>
          <h3>{{ t("placeholder.capabilityTitle") }}</h3>
          <p>{{ t("common.protectedRoute") }}</p>
        </div>
      </div>
      <ul class="status-list">
        <li>{{ t("placeholder.capabilityOne") }}</li>
        <li>{{ t("placeholder.capabilityTwo") }}</li>
        <li>{{ t("placeholder.capabilityThree") }}</li>
        <li>{{ t("placeholder.capabilityFour") }}</li>
      </ul>
    </article>

    <article class="card placeholder-card">
      <div class="card__header card__header--tight">
        <div>
          <h3>{{ t("placeholder.nextTitle") }}</h3>
          <p>{{ t("placeholder.permissionDescription") }}</p>
        </div>
      </div>
      <p>{{ t("placeholder.nextDescription", { milestone: milestoneLabel }) }}</p>

      <div class="placeholder-inline-note">
        <span class="pill">{{ t("common.componentGuard") }}</span>
      </div>

      <PermissionGuard permission="audit.export">
        <p class="placeholder-note">{{ t("placeholder.exportHint") }}</p>
        <template #fallback>
          <p class="placeholder-note placeholder-note--muted">{{ t("placeholder.exportHintFallback") }}</p>
        </template>
      </PermissionGuard>
    </article>
  </section>
</template>
