import type { PermissionCode } from "@/shared/permissions";

export type Milestone = "T13" | "T14" | "T15";

export interface PrimaryNavItem {
  key: "dashboard" | "domains" | "assets" | "caAccounts" | "delivery" | "discoveries" | "jobs" | "audit" | "settings";
  routeName:
    | "dashboard"
    | "domains"
    | "assets"
    | "ca-accounts"
    | "delivery"
    | "discoveries"
    | "jobs"
    | "audit"
    | "settings-webhooks";
  labelKey: string;
  counterKey?: string;
}

export interface ProtectedPageDefinition {
  name: string;
  path: string;
  titleKey: string;
  descriptionKey: string;
  sectionKey: string;
  milestone: Milestone;
  navKey?: PrimaryNavItem["key"];
  permission?: PermissionCode | PermissionCode[];
}

export const defaultProtectedRouteName = "dashboard";

export const primaryNavItems: PrimaryNavItem[] = [
  { key: "dashboard", routeName: "dashboard", labelKey: "navigation.dashboard" },
  { key: "domains", routeName: "domains", labelKey: "navigation.domains" },
  { key: "assets", routeName: "assets", labelKey: "navigation.assets", counterKey: "assets" },
  { key: "caAccounts", routeName: "ca-accounts", labelKey: "navigation.caAccounts" },
  { key: "delivery", routeName: "delivery", labelKey: "navigation.delivery", counterKey: "delivery" },
  { key: "discoveries", routeName: "discoveries", labelKey: "navigation.discoveries", counterKey: "discoveries" },
  { key: "jobs", routeName: "jobs", labelKey: "navigation.jobs", counterKey: "jobs" },
  { key: "audit", routeName: "audit", labelKey: "navigation.audit" },
  { key: "settings", routeName: "settings-webhooks", labelKey: "navigation.settings" },
];

export const protectedPageDefinitions: ProtectedPageDefinition[] = [
  {
    name: "dashboard",
    path: "dashboard",
    titleKey: "pages.dashboard.title",
    descriptionKey: "pages.dashboard.description",
    sectionKey: "sections.operations",
    milestone: "T15",
    navKey: "dashboard",
  },
  {
    name: "domains",
    path: "domains",
    titleKey: "pages.domains.title",
    descriptionKey: "pages.domains.description",
    sectionKey: "sections.governance",
    milestone: "T14",
    navKey: "domains",
  },
  {
    name: "domain-detail",
    path: "domains/:id",
    titleKey: "pages.domainDetail.title",
    descriptionKey: "pages.domainDetail.description",
    sectionKey: "sections.governance",
    milestone: "T14",
    navKey: "domains",
  },
  {
    name: "assets",
    path: "assets",
    titleKey: "pages.assets.title",
    descriptionKey: "pages.assets.description",
    sectionKey: "sections.operations",
    milestone: "T15",
    navKey: "assets",
  },
  {
    name: "asset-apply",
    path: "assets/apply",
    titleKey: "pages.assetApply.title",
    descriptionKey: "pages.assetApply.description",
    sectionKey: "sections.operations",
    milestone: "T15",
    navKey: "assets",
  },
  {
    name: "asset-request-result",
    path: "assets/requests/:id/result",
    titleKey: "pages.assetRequestResult.title",
    descriptionKey: "pages.assetRequestResult.description",
    sectionKey: "sections.operations",
    milestone: "T15",
    navKey: "assets",
  },
  {
    name: "asset-detail",
    path: "assets/:id",
    titleKey: "pages.assetDetail.title",
    descriptionKey: "pages.assetDetail.description",
    sectionKey: "sections.operations",
    milestone: "T15",
    navKey: "assets",
  },
  {
    name: "ca-accounts",
    path: "ca-accounts",
    titleKey: "pages.caAccounts.title",
    descriptionKey: "pages.caAccounts.description",
    sectionKey: "sections.governance",
    milestone: "T14",
    navKey: "caAccounts",
  },
  {
    name: "ca-account-detail",
    path: "ca-accounts/:id",
    titleKey: "pages.caAccountDetail.title",
    descriptionKey: "pages.caAccountDetail.description",
    sectionKey: "sections.governance",
    milestone: "T14",
    navKey: "caAccounts",
  },
  {
    name: "delivery",
    path: "delivery",
    titleKey: "pages.deliveryWorkspace.title",
    descriptionKey: "pages.deliveryWorkspace.description",
    sectionKey: "sections.execution",
    milestone: "T14",
    navKey: "delivery",
  },
  {
    name: "delivery-targets",
    path: "delivery/targets",
    titleKey: "pages.deliveryTargets.title",
    descriptionKey: "pages.deliveryTargets.description",
    sectionKey: "sections.execution",
    milestone: "T14",
    navKey: "delivery",
  },
  {
    name: "delivery-target-detail",
    path: "delivery/targets/:id",
    titleKey: "pages.deliveryTargetDetail.title",
    descriptionKey: "pages.deliveryTargetDetail.description",
    sectionKey: "sections.execution",
    milestone: "T14",
    navKey: "delivery",
  },
  {
    name: "delivery-nodes",
    path: "delivery/nodes",
    titleKey: "pages.deliveryNodes.title",
    descriptionKey: "pages.deliveryNodes.description",
    sectionKey: "sections.execution",
    milestone: "T14",
    navKey: "delivery",
  },
  {
    name: "delivery-node-detail",
    path: "delivery/nodes/:id",
    titleKey: "pages.deliveryNodeDetail.title",
    descriptionKey: "pages.deliveryNodeDetail.description",
    sectionKey: "sections.execution",
    milestone: "T14",
    navKey: "delivery",
  },
  {
    name: "discoveries",
    path: "discoveries",
    titleKey: "pages.discoveries.title",
    descriptionKey: "pages.discoveries.description",
    sectionKey: "sections.operations",
    milestone: "T15",
    navKey: "discoveries",
  },
  {
    name: "discovery-detail",
    path: "discoveries/:id",
    titleKey: "pages.discoveryDetail.title",
    descriptionKey: "pages.discoveryDetail.description",
    sectionKey: "sections.operations",
    milestone: "T15",
    navKey: "discoveries",
  },
  {
    name: "jobs",
    path: "jobs",
    titleKey: "pages.jobs.title",
    descriptionKey: "pages.jobs.description",
    sectionKey: "sections.operations",
    milestone: "T15",
    navKey: "jobs",
  },
  {
    name: "job-detail",
    path: "jobs/:id",
    titleKey: "pages.jobDetail.title",
    descriptionKey: "pages.jobDetail.description",
    sectionKey: "sections.operations",
    milestone: "T15",
    navKey: "jobs",
  },
  {
    name: "audit",
    path: "audit",
    titleKey: "pages.audit.title",
    descriptionKey: "pages.audit.description",
    sectionKey: "sections.security",
    milestone: "T15",
    navKey: "audit",
    permission: "audit.read",
  },
  {
    name: "audit-detail",
    path: "audit/:id",
    titleKey: "pages.auditDetail.title",
    descriptionKey: "pages.auditDetail.description",
    sectionKey: "sections.security",
    milestone: "T15",
    navKey: "audit",
    permission: "audit.read",
  },
  {
    name: "settings-webhooks",
    path: "settings/webhooks",
    titleKey: "pages.settingsWebhooks.title",
    descriptionKey: "pages.settingsWebhooks.description",
    sectionKey: "sections.settings",
    milestone: "T14",
    navKey: "settings",
    permission: "settings.read",
  },
  {
    name: "settings-renewal-window",
    path: "settings/renewal-window",
    titleKey: "pages.settingsRenewalWindow.title",
    descriptionKey: "pages.settingsRenewalWindow.description",
    sectionKey: "sections.settings",
    milestone: "T14",
    navKey: "settings",
    permission: "settings.read",
  },
  {
    name: "settings-security",
    path: "settings/security",
    titleKey: "pages.settingsSecurity.title",
    descriptionKey: "pages.settingsSecurity.description",
    sectionKey: "sections.settings",
    milestone: "T14",
    navKey: "settings",
    permission: "settings.read",
  },
];
