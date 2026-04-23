<script setup lang="ts">
import { reactive, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useI18n } from "vue-i18n";

import { ApiError } from "@/shared/api/client";
import { useSessionStore } from "@/shared/auth/store";
import type { SupportedLocale } from "@/shared/i18n";

const route = useRoute();
const router = useRouter();
const { t } = useI18n();
const sessionStore = useSessionStore();
const appVersion = __APP_VERSION__;
const showDevHints = import.meta.env.DEV;

const form = reactive({
  username: import.meta.env.DEV ? "admin" : "",
  password: import.meta.env.DEV ? "admin123!" : "",
});

const submitting = ref(false);
const errorMessage = ref("");

async function onSubmit(): Promise<void> {
  submitting.value = true;
  errorMessage.value = "";

  try {
    await sessionStore.login(form.username, form.password);
    const redirect = typeof route.query.redirect === "string" ? route.query.redirect : "/dashboard";
    await router.replace(redirect);
  } catch (error) {
    if (error instanceof ApiError && (error.status === 401 || error.code === "unauthorized")) {
      errorMessage.value = t("login.invalid");
    } else {
      errorMessage.value = t("login.genericError");
    }
  } finally {
    submitting.value = false;
  }
}

async function onLocaleChange(locale: SupportedLocale): Promise<void> {
  await sessionStore.changeLocale(locale);
}
</script>

<template>
  <section class="login-shell">
    <article class="login-story card">
      <div class="login-story__header">
        <span class="brand__eyebrow">{{ t("app.brandEyebrow") }}</span>
        <div class="locale-switch">
          <button
            v-for="locale in sessionStore.availableLocales"
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

      <div class="login-story__body">
        <h1>{{ t("app.brandTitle") }}</h1>
        <p>{{ t("app.brandDescription") }}</p>

        <div class="detail-stack">
          <div class="detail-row">
            <span>{{ t("shell.versionLabel") }}</span>
            <strong class="mono">v{{ appVersion }}</strong>
          </div>
          <div class="detail-row">
            <span>{{ t("shell.languageLabel") }}</span>
            <strong>{{ sessionStore.locale }}</strong>
          </div>
        </div>
      </div>
    </article>

    <article class="login-panel card">
      <div class="card__header card__header--tight">
        <div>
          <h2>{{ t("login.title") }}</h2>
          <p>{{ t("login.subtitle") }}</p>
        </div>
      </div>

      <form class="login-form" @submit.prevent="onSubmit">
        <label class="form-field">
          <span>{{ t("login.username") }}</span>
          <input v-model.trim="form.username" class="input" name="username" autocomplete="username" />
        </label>

        <label class="form-field">
          <span>{{ t("login.password") }}</span>
          <input
            v-model="form.password"
            class="input"
            type="password"
            name="password"
            autocomplete="current-password"
          />
        </label>

        <p v-if="errorMessage" class="form-error">{{ errorMessage }}</p>

        <button class="button button--primary login-submit" type="submit" :disabled="submitting">
          {{ submitting ? t("login.submitting") : t("login.submit") }}
        </button>
      </form>

      <div v-if="showDevHints" class="login-hints">
        <h3>{{ t("login.devAccounts") }}</h3>
        <p class="mono">{{ t("login.adminHint") }}</p>
        <p class="mono">{{ t("login.auditorHint") }}</p>
      </div>

      <p class="login-note">{{ t("login.localeHint") }}</p>
    </article>
  </section>
</template>
