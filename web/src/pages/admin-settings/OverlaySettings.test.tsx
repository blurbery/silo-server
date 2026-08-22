import type { ReactNode } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import OverlaySettings from "./OverlaySettings";
import { WEB_WATCHED_INDICATOR_SETTING_KEY } from "@/lib/watchedIndicator";

const mocks = vi.hoisted(() => ({
  setValue: vi.fn(),
  useSettingsForm: vi.fn(),
}));

const values: Record<string, string> = {
  "overlays.enabled": "true",
  "defaults.card_overlays": "",
  [WEB_WATCHED_INDICATOR_SETTING_KEY]: "pill",
};

vi.mock("@/hooks/useSettingsForm", () => ({
  useSettingsForm: (options: { keys: string[] }) => mocks.useSettingsForm(options),
}));

vi.mock("@/components/ui/select", () => ({
  Select: ({
    children,
    value,
    onValueChange,
  }: {
    children: ReactNode;
    value?: string;
    onValueChange?: (value: string) => void;
  }) => (
    <div>
      {children}
      {value === "pill" ? (
        <button type="button" onClick={() => onValueChange?.("eye")}>
          Choose eye indicator
        </button>
      ) : null}
    </div>
  ),
  SelectContent: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  SelectItem: ({ children, value }: { children: ReactNode; value: string }) => (
    <div data-value={value}>{children}</div>
  ),
  SelectTrigger: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  SelectValue: () => null,
}));

vi.mock("@/components/ui/switch", () => ({
  Switch: () => <button type="button">Toggle</button>,
}));

function makeForm() {
  return {
    isLoading: false,
    getValue: (key: string) => values[key] ?? "",
    setValue: mocks.setValue,
    dirtyCount: 0,
    dirtyKeys: [],
    isDirty: vi.fn(() => false),
    save: vi.fn(),
    discard: vi.fn(),
    isSaving: false,
    restartRequired: false,
    sensitiveConfigured: [],
    sensitiveManagedByEnv: [],
    buildConnectionCheckRequest: vi.fn(),
  };
}

function renderPage() {
  return renderToStaticMarkup(<OverlaySettings />);
}

describe("OverlaySettings watched indicator", () => {
  beforeEach(() => {
    values["overlays.enabled"] = "true";
    values[WEB_WATCHED_INDICATOR_SETTING_KEY] = "pill";
    mocks.setValue.mockReset();
    mocks.useSettingsForm.mockReset();
    mocks.useSettingsForm.mockReturnValue(makeForm());
  });

  it("registers the global web setting and shows every supported choice", () => {
    const markup = renderPage();

    expect(mocks.useSettingsForm).toHaveBeenCalledWith({
      keys: expect.arrayContaining([WEB_WATCHED_INDICATOR_SETTING_KEY]),
    });
    for (const label of [
      "Rounded pill",
      "Square outline",
      "Text only",
      "Text + eye",
      "Text + check",
      "None",
    ]) {
      expect(markup).toContain(label);
    }
    expect(markup).toContain("applies server-wide");
  });

  it("previews the selected style below the poster", () => {
    const markup = renderPage();

    expect(markup.indexOf("Movie preview")).toBeLessThan(markup.indexOf("Example Movie"));
    expect(markup).toContain('data-watched-indicator="pill"');
  });

  it("writes the selected web style through the shared settings form", async () => {
    render(<OverlaySettings />);

    await userEvent.click(screen.getByRole("button", { name: "Choose eye indicator" }));

    expect(mocks.setValue).toHaveBeenCalledWith(WEB_WATCHED_INDICATOR_SETTING_KEY, "eye");
  });

  it("keeps the watched control active when poster overlays are disabled", () => {
    values["overlays.enabled"] = "false";
    const markup = renderPage();

    expect(markup).toContain("Watched indicator");
    expect(markup).toContain('data-watched-indicator="pill"');
  });

  it("supports hiding the web indicator without removing the preview caption", () => {
    values[WEB_WATCHED_INDICATOR_SETTING_KEY] = "none";
    const markup = renderPage();

    expect(markup).toContain("Example Movie");
    expect(markup).not.toContain("data-watched-indicator");
  });
});
