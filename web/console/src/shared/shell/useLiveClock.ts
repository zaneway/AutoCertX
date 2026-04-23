import { computed, onBeforeUnmount, onMounted, ref } from "vue";

export function useLiveClock(resolveLocale: () => string) {
  const now = ref(new Date());
  let timer: number | undefined;

  onMounted(() => {
    timer = window.setInterval(() => {
      now.value = new Date();
    }, 1000);
  });

  onBeforeUnmount(() => {
    if (timer !== undefined) {
      window.clearInterval(timer);
    }
  });

  const formatted = computed(() =>
    new Intl.DateTimeFormat(resolveLocale(), {
      dateStyle: "medium",
      timeStyle: "medium",
    }).format(now.value),
  );

  return {
    now,
    formatted,
  };
}
