import { useQuery } from "@tanstack/vue-query";
import { computed } from "vue";

import { fetchNavigationCounters } from "@/shared/api/statistics";
import { useSessionStore } from "@/shared/auth/store";
import { queryKeys } from "@/shared/query/keys";

export function useShellSummaryQuery() {
  const sessionStore = useSessionStore();

  const scopeKey = computed(() => {
    if (!sessionStore.context) {
      return "anonymous";
    }

    return `${sessionStore.context.tenant.id}:${sessionStore.context.project.id}:${sessionStore.context.environment.id}`;
  });

  return useQuery({
    queryKey: computed(() => queryKeys.shell.summary(scopeKey.value)),
    queryFn: fetchNavigationCounters,
    enabled: computed(() => sessionStore.isAuthenticated),
    staleTime: 30_000,
    retry: false,
  });
}
