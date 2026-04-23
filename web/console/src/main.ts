import { QueryClient, VueQueryPlugin } from "@tanstack/vue-query";
import { createPinia } from "pinia";
import { createApp } from "vue";

import App from "@/app/App.vue";
import { createAppRouter } from "@/app/router";
import "@/app/styles/base.css";
import { useSessionStore } from "@/shared/auth/store";
import { i18n } from "@/shared/i18n";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      retry: 1,
      refetchOnWindowFocus: false,
    },
  },
});

const app = createApp(App);
const pinia = createPinia();

app.use(pinia);
app.use(i18n);
app.use(VueQueryPlugin, { queryClient });

async function bootstrap(): Promise<void> {
  const sessionStore = useSessionStore(pinia);
  await sessionStore.bootstrap();

  const router = createAppRouter(pinia);
  app.use(router);

  await router.isReady();
  app.mount("#app");
}

void bootstrap();
