import "vue-router";

import type { Milestone } from "@/shared/navigation/catalog";
import type { PermissionCode } from "@/shared/permissions";

declare module "vue-router" {
  interface RouteMeta {
    requiresAuth?: boolean;
    publicOnly?: boolean;
    titleKey?: string;
    descriptionKey?: string;
    sectionKey?: string;
    milestone?: Milestone;
    navKey?: string;
    permission?: PermissionCode | PermissionCode[];
  }
}
