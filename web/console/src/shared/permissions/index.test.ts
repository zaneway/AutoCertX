import { describe, expect, it } from "vitest";

import { expandPermissions, hasPermission } from "@/shared/permissions";

describe("permission mapping", () => {
  it("grants wildcard access to tenant admins", () => {
    expect(expandPermissions(["tenant_admin"])).toContain("*");
    expect(hasPermission(["tenant_admin"], "settings.write")).toBe(true);
  });

  it("grants audit export only to roles that explicitly carry it", () => {
    expect(hasPermission(["auditor"], "audit.export")).toBe(true);
    expect(hasPermission(["platform_engineer"], "audit.export")).toBe(false);
  });
});
