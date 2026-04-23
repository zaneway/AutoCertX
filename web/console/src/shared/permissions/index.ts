import { computed } from "vue";

import { useSessionStore } from "@/shared/auth/store";

export type PermissionCode =
  | "*"
  | "auth.context.read"
  | "auth.preferences.write"
  | "audit.read"
  | "audit.export"
  | "settings.read"
  | "settings.write";

const rolePermissionMatrix: Record<string, PermissionCode[]> = {
  tenant_admin: ["*"],
  security_admin: ["auth.context.read", "auth.preferences.write", "audit.read", "settings.read", "settings.write"],
  platform_engineer: ["auth.context.read", "auth.preferences.write", "audit.read", "settings.read", "settings.write"],
  auditor: ["auth.context.read", "audit.read", "audit.export"],
};

function normalizePermissions(required?: PermissionCode | PermissionCode[]): PermissionCode[] {
  if (!required) {
    return [];
  }
  return Array.isArray(required) ? required : [required];
}

export function expandPermissions(roleCodes: readonly string[]): PermissionCode[] {
  const expanded = new Set<PermissionCode>();
  roleCodes.forEach((roleCode) => {
    (rolePermissionMatrix[roleCode] ?? []).forEach((permission) => expanded.add(permission));
  });
  return [...expanded];
}

export function hasPermission(roleCodes: readonly string[], required?: PermissionCode | PermissionCode[]): boolean {
  const requested = normalizePermissions(required);
  if (requested.length === 0) {
    return true;
  }

  const expanded = new Set(expandPermissions(roleCodes));
  if (expanded.has("*")) {
    return true;
  }

  return requested.some((permission) => expanded.has(permission));
}

export function usePermission(required?: PermissionCode | PermissionCode[]) {
  const sessionStore = useSessionStore();

  return {
    allowed: computed(() => hasPermission(sessionStore.roles, required)),
  };
}
