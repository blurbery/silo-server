import type { ReactNode } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import OverlaySettings from "./OverlaySettings";

const mocks = vi.hoisted(() => ({
  setValue: vi.fn(),
  useSettingsForm: vi.fn(),
}));

const values: Record<string, string> = {
  "defaults.card_quick_actions_enabled": "true",
  "defaults.card_quick_actions": "both",
  "overlays.enabled": "true",
  "defaults.card_overlays": "",
};

vi.mock("@/hooks/useSettingsForm", () => ({
  useSettingsForm: (options: { keys: string[] }) => mocks.useSettingsForm(options),
}));

vi.mock("@/components/ui/select", () => ({
  Select: ({
    children,
    disabled = false,
    onValueChange,
    value,
  }: {
    children: ReactNode;
    disabled?: boolean;
    onValueChange?: (value: string) => void;
    value?: string;
  }) => (
    <div>
      <button
        type="button"
        data-testid="select-control"
        disabled={disabled}
        onClick={() => onValueChange?.(value ?? "")}
      >
        Select
      </button>
      {children}
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
  Switch: ({
    checked,
    disabled = false,
    onCheckedChange,
  }: {
    checked: boolean;
    disabled?: boolean;
    onCheckedChange?: (checked: boolean) => void;
  }) => (
    <button
      type="button"
      data-testid="overlay-switch"
      disabled={disabled}
      onClick={() => onCheckedChange?.(!checked)}
    >
      Toggle
    </button>
  ),
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

describe("OverlaySettings", () => {
  beforeEach(() => {
    values["overlays.enabled"] = "true";
    values["defaults.card_quick_actions_enabled"] = "true";
    values["defaults.card_quick_actions"] = "both";
    mocks.setValue.mockReset();
    mocks.useSettingsForm.mockReset();
    mocks.useSettingsForm.mockReturnValue(makeForm());
  });

  it("registers card quick-action defaults above the overlay controls", () => {
    const markup = renderPage();

    expect(mocks.useSettingsForm).toHaveBeenCalledWith({
      keys: [
        "defaults.card_quick_actions_enabled",
        "defaults.card_quick_actions",
        "overlays.enabled",
        "defaults.card_overlays",
      ],
    });
    expect(markup).toContain("Card quick actions");
    expect(markup).toContain("When disabled, no profile can show them.");
    expect(markup).toContain("Favorites only");
    expect(markup).toContain("Watch indicator only");
    expect(markup.indexOf("Card quick actions")).toBeLessThan(
      markup.indexOf("Card Overlays Enabled"),
    );
    expect(markup).toContain("Default style preset");
  });

  it("keeps the preset selector at its previous responsive column width", () => {
    const markup = renderPage();

    expect(markup).toContain("sm:grid-cols-2");
  });

  it("previews poster overlays without watched metadata", () => {
    const markup = renderPage();

    expect(markup).toContain("Movie preview");
    expect(markup).not.toContain("Example Movie");
    expect(markup).not.toContain("data-watched-indicator");
  });

  it("keeps the overlay preview but hides its badges when overlays are disabled", () => {
    values["overlays.enabled"] = "false";
    const markup = renderPage();

    expect(markup).toContain("Movie preview");
    expect(markup).not.toContain("data-overlay-id");
  });

  it("prevents disabled overlay controls from changing defaults", () => {
    values["overlays.enabled"] = "false";
    render(<OverlaySettings />);

    const selectControls = screen.getAllByTestId("select-control");
    const overlaySwitches = screen.getAllByTestId("overlay-switch");
    const defaultOverlaySwitches = overlaySwitches.slice(2);

    expect(selectControls[0]).not.toHaveAttribute("disabled");
    expect(selectControls.slice(1).every((control) => control.hasAttribute("disabled"))).toBe(true);
    expect(overlaySwitches[0]).not.toHaveAttribute("disabled");
    expect(overlaySwitches[1]).not.toHaveAttribute("disabled");
    expect(defaultOverlaySwitches.every((control) => control.hasAttribute("disabled"))).toBe(true);

    selectControls.slice(1).forEach((control) => fireEvent.click(control));
    defaultOverlaySwitches.forEach((control) => fireEvent.click(control));

    expect(mocks.setValue).not.toHaveBeenCalled();
  });

  it("disables only the quick-action dropdown when its default toggle is off", () => {
    values["defaults.card_quick_actions_enabled"] = "false";
    render(<OverlaySettings />);

    const selectControls = screen.getAllByTestId("select-control");

    expect(selectControls[0]).toHaveAttribute("disabled");
    expect(selectControls.slice(1).some((control) => !control.hasAttribute("disabled"))).toBe(true);
  });
});
