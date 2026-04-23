import { ApiError, apiClient } from "@/shared/api/client";

export type NavigationCounterMap = Partial<Record<string, number>>;

interface StatisticsSummaryPayload {
  navigation_counters?: NavigationCounterMap;
  data?: {
    navigation_counters?: NavigationCounterMap;
  };
}

export async function fetchNavigationCounters(): Promise<NavigationCounterMap> {
  try {
    const payload = await apiClient.request<StatisticsSummaryPayload>("/api/v1/statistics/summary");
    return payload.navigation_counters ?? payload.data?.navigation_counters ?? {};
  } catch (error) {
    if (error instanceof ApiError && (error.status === 404 || error.status === 501)) {
      return {};
    }
    throw error;
  }
}
