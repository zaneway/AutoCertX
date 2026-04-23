import type { Pinia } from "pinia";
import { createRouter, createWebHistory, type RouteRecordRaw } from "vue-router";

import ConsoleLayout from "@/app/layouts/ConsoleLayout.vue";
import PublicLayout from "@/app/layouts/PublicLayout.vue";
import ForbiddenPage from "@/app/pages/ForbiddenPage.vue";
import LoginPage from "@/app/pages/LoginPage.vue";
import NotFoundPage from "@/app/pages/NotFoundPage.vue";
import PlaceholderPage from "@/app/pages/PlaceholderPage.vue";
import { useSessionStore } from "@/shared/auth/store";
import { i18n } from "@/shared/i18n";
import { defaultProtectedRouteName, protectedPageDefinitions } from "@/shared/navigation/catalog";
import { hasPermission } from "@/shared/permissions";

const protectedChildRoutes: RouteRecordRaw[] = protectedPageDefinitions.map((page) => ({
  path: page.path,
  name: page.name,
  component: PlaceholderPage,
  meta: {
    requiresAuth: true,
    titleKey: page.titleKey,
    descriptionKey: page.descriptionKey,
    sectionKey: page.sectionKey,
    milestone: page.milestone,
    navKey: page.navKey,
    permission: page.permission,
  },
}));

const routes: RouteRecordRaw[] = [
  {
    path: "/login",
    component: PublicLayout,
    children: [
      {
        path: "",
        name: "login",
        component: LoginPage,
        meta: {
          publicOnly: true,
          sectionKey: "sections.platform",
        },
      },
    ],
  },
  {
    path: "/",
    component: ConsoleLayout,
    children: [
      {
        path: "",
        redirect: { name: defaultProtectedRouteName },
      },
      ...protectedChildRoutes,
      {
        path: "forbidden",
        name: "forbidden",
        component: ForbiddenPage,
        meta: {
          requiresAuth: true,
          titleKey: "pages.forbidden.title",
          descriptionKey: "pages.forbidden.description",
          sectionKey: "sections.platform",
          milestone: "T13",
        },
      },
    ],
  },
  {
    path: "/:pathMatch(.*)*",
    name: "not-found",
    component: NotFoundPage,
    meta: {
      titleKey: "pages.notFound.title",
      descriptionKey: "pages.notFound.description",
      sectionKey: "sections.platform",
      milestone: "T13",
    },
  },
];

export function createAppRouter(pinia: Pinia) {
  const router = createRouter({
    history: createWebHistory(import.meta.env.BASE_URL),
    routes,
  });

  router.beforeEach(async (to) => {
    const sessionStore = useSessionStore(pinia);
    if (!sessionStore.bootstrapped) {
      await sessionStore.bootstrap();
    }

    if (to.meta.publicOnly && sessionStore.isAuthenticated) {
      return { name: defaultProtectedRouteName };
    }

    if (!to.meta.requiresAuth) {
      return true;
    }

    const authenticated = await sessionStore.ensureAuthenticated();
    if (!authenticated) {
      return {
        name: "login",
        query: { redirect: to.fullPath },
      };
    }

    if (to.meta.permission && !hasPermission(sessionStore.roles, to.meta.permission)) {
      return { name: "forbidden" };
    }

    return true;
  });

  router.afterEach((to) => {
    const pageTitle = to.meta.titleKey ? i18n.global.t(String(to.meta.titleKey)) : i18n.global.t("app.brandTitle");
    document.title = `${pageTitle} | AutoCertX`;
  });

  return router;
}
