import { describe, expect, it } from "vitest";

import { primaryNavItems, protectedPageDefinitions } from "@/shared/navigation/catalog";

describe("navigation catalog", () => {
  it("maps every primary nav entry to a protected page", () => {
    const routeNames = new Set(protectedPageDefinitions.map((page) => page.name));

    primaryNavItems.forEach((item) => {
      expect(routeNames.has(item.routeName)).toBe(true);
    });
  });

  it("assigns a milestone to every protected page", () => {
    protectedPageDefinitions.forEach((page) => {
      expect(["T13", "T14", "T15"]).toContain(page.milestone);
    });
  });
});
