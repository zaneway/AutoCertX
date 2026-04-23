export const queryKeys = {
  auth: {
    context: ["auth", "context"] as const,
  },
  shell: {
    summary(scope: string) {
      return ["shell", "summary", scope] as const;
    },
  },
};
