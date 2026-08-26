import type { ReactNode } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import CardOverlaySettings from "./CardOverlaySettings";

vi.mock("@/hooks/useOverlayPrefs", () => ({
  useOverlayPrefs: () => ({
    prefs: null,
    setPrefs: vi.fn(),
    quickActionPreference: "favorites",
    setQuickActionMode: vi.fn(),
    quickActionsEnabled: false,
    quickActionsGloballyEnabled: false,
    setQuickActionsEnabled: vi.fn(),
    isLoading: false,
    enabled: true,
  }),
}));

vi.mock("@/components/overlays/OverlayPreviewCard", () => ({
  OverlayPreviewCard: () => <div>Overlay preview</div>,
}));

vi.mock("@/components/ui/select", () => ({
  Select: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  SelectContent: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  SelectItem: ({ children, value }: { children: ReactNode; value: string }) => (
    <div data-value={value}>{children}</div>
  ),
  SelectTrigger: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  SelectValue: () => null,
}));

describe("CardOverlaySettings", () => {
  it("places the profile quick-action override above the overlay preview", () => {
    const markup = renderToStaticMarkup(<CardOverlaySettings />);

    expect(markup).toContain("Override the server defaults for this profile.");
    expect(markup).toContain("Card quick actions");
    expect(markup).toContain("Both");
    expect(markup).toContain("Favorites only");
    expect(markup).toContain("Watch indicator only");
    expect(markup.indexOf("Card quick actions")).toBeLessThan(markup.indexOf("Overlay preview"));
    expect(markup).toContain('aria-label="Enable card quick actions"');
    expect(markup).toContain("Card quick actions have been disabled by your server administrator.");
    expect(markup).toContain('disabled=""');
  });
});
