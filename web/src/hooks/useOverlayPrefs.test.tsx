import { createElement, type ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  api: vi.fn(),
  setValue: vi.fn(),
  effective: undefined as Record<string, { value: unknown }> | undefined,
  profileId: null as string | null,
}));

vi.mock("@/api/client", () => ({
  api: mocks.api,
}));

// Keep the real query-key builder and error taxonomy so the hook's optimistic
// cache writes are exercised against the actual key shape; only the two hooks
// that reach the network are replaced.
vi.mock("@/hooks/queries/settingValues", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/hooks/queries/settingValues")>()),
  useEffectiveSettings: () => ({ data: mocks.effective, isLoading: false }),
  useSetSettingValue: () => ({ mutate: mocks.setValue }),
}));

vi.mock("@/utils/storage", () => ({
  storage: {
    KEYS: { PROFILE_ID: "profile_id" },
    get: () => mocks.profileId,
  },
}));

import { useOverlayPrefs } from "./useOverlayPrefs";
import { useUpdateServerSettings } from "./queries/admin/settings";

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return function Wrapper({ children }: { children: ReactNode }) {
    return createElement(QueryClientProvider, { client: queryClient }, children);
  };
}

describe("useOverlayPrefs", () => {
  beforeEach(() => {
    mocks.api.mockReset();
    mocks.setValue.mockReset();
    mocks.effective = undefined;
    mocks.profileId = null;
  });

  afterEach(cleanup);

  it("reads the server-wide overlay configuration without bypassing the shared query cache", async () => {
    mocks.api.mockResolvedValue({ enabled: true });
    const { result } = renderHook(() => useOverlayPrefs(), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(mocks.api).toHaveBeenCalledWith("/settings/overlay-config");
    expect(result.current.overlaysEnabled).toBe(true);
    expect(result.current.prefs).not.toBeNull();
    expect(result.current).not.toHaveProperty("enabled");
    expect(result.current.quickActionsEnabled).toBe(false);
    expect(result.current).not.toHaveProperty("quickActionsGloballyEnabled");
    expect(result.current.quickActionPreference).toBe("both");
    expect(result.current.quickActionMode).toBe("none");
    expect(result.current).not.toHaveProperty("watchedIndicatorStyle");
  });

  it("uses disabled and mode values from the admin defaults for an unset profile", async () => {
    mocks.api.mockResolvedValue({
      enabled: true,
      quick_actions_enabled: false,
      quick_actions_default: "favorites",
    });
    const { result } = renderHook(() => useOverlayPrefs(), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(result.current.quickActionsEnabled).toBe(false);
    expect(result.current.quickActionPreference).toBe("favorites");
    expect(result.current.quickActionMode).toBe("none");
  });

  it("inherits an enabled admin default for a profile that has not chosen", async () => {
    mocks.profileId = "profile-1";
    mocks.effective = {};
    mocks.api.mockResolvedValue({
      enabled: true,
      quick_actions_enabled: true,
      quick_actions_default: "favorites",
    });
    const { result } = renderHook(() => useOverlayPrefs(), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(result.current.quickActionsEnabled).toBe(true);
    expect(result.current.quickActionMode).toBe("favorites");
  });

  it("lets a profile opt in to quick actions while the admin default is off", async () => {
    mocks.profileId = "profile-1";
    mocks.effective = {
      "ui.card_quick_actions": { value: "watched" },
      "ui.card_quick_actions_enabled": { value: true },
    };
    mocks.api.mockResolvedValue({
      enabled: true,
      quick_actions_enabled: false,
      quick_actions_default: "favorites",
    });
    const { result } = renderHook(() => useOverlayPrefs(), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(result.current.quickActionsEnabled).toBe(true);
    expect(result.current.quickActionPreference).toBe("watched");
    expect(result.current.quickActionMode).toBe("watched");

    act(() => result.current.setQuickActionsEnabled(false));
    act(() => result.current.setQuickActionMode("both"));
    expect(mocks.setValue).toHaveBeenCalledWith(
      {
        key: "ui.card_quick_actions_enabled",
        value: false,
        identity: { scope: "profile" },
      },
      expect.objectContaining({ onError: expect.any(Function) }),
    );
    expect(mocks.setValue).toHaveBeenCalledWith(
      {
        key: "ui.card_quick_actions",
        value: "both",
        identity: { scope: "profile" },
      },
      expect.objectContaining({ onError: expect.any(Function) }),
    );
  });

  it("lets a profile opt out of quick actions while the admin default is on", async () => {
    mocks.profileId = "profile-1";
    mocks.effective = {
      "ui.card_quick_actions": { value: "watched" },
      "ui.card_quick_actions_enabled": { value: false },
    };
    mocks.api.mockResolvedValue({
      enabled: true,
      quick_actions_enabled: true,
      quick_actions_default: "favorites",
    });
    const { result } = renderHook(() => useOverlayPrefs(), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(result.current.quickActionsEnabled).toBe(false);
    expect(result.current.quickActionPreference).toBe("watched");
    expect(result.current.quickActionMode).toBe("none");

    act(() => result.current.setQuickActionsEnabled(true));
    act(() => result.current.setQuickActionMode("both"));
    expect(mocks.setValue).toHaveBeenCalledWith(
      {
        key: "ui.card_quick_actions_enabled",
        value: true,
        identity: { scope: "profile" },
      },
      expect.objectContaining({ onError: expect.any(Function) }),
    );
    expect(mocks.setValue).toHaveBeenCalledWith(
      {
        key: "ui.card_quick_actions",
        value: "both",
        identity: { scope: "profile" },
      },
      expect.objectContaining({ onError: expect.any(Function) }),
    );
  });

  it("inherits a disabled overlay default for a profile that has not chosen", async () => {
    mocks.profileId = "profile-1";
    mocks.effective = {};
    mocks.api.mockResolvedValue({ enabled: false });
    const { result } = renderHook(() => useOverlayPrefs(), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(result.current.overlaysEnabled).toBe(false);
    expect(result.current.prefs).toBeNull();
  });

  it("lets a profile opt in to overlays while the server default is off", async () => {
    mocks.profileId = "profile-1";
    mocks.effective = { "ui.card_overlays_enabled": { value: true } };
    mocks.api.mockResolvedValue({ enabled: false });
    const { result } = renderHook(() => useOverlayPrefs(), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(result.current.overlaysEnabled).toBe(true);
    expect(result.current.prefs).not.toBeNull();

    act(() => result.current.setOverlaysEnabled(false));
    expect(mocks.setValue).toHaveBeenCalledWith(
      {
        key: "ui.card_overlays_enabled",
        value: false,
        identity: { scope: "profile" },
      },
      expect.objectContaining({ onError: expect.any(Function) }),
    );
  });

  it("lets a profile opt out of overlays while the server default is on", async () => {
    mocks.profileId = "profile-1";
    mocks.effective = { "ui.card_overlays_enabled": { value: false } };
    mocks.api.mockResolvedValue({ enabled: true });
    const { result } = renderHook(() => useOverlayPrefs(), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(result.current.overlaysEnabled).toBe(false);
    expect(result.current.prefs).toBeNull();

    act(() => result.current.setOverlaysEnabled(false));
    expect(mocks.setValue).not.toHaveBeenCalled();

    act(() => result.current.setOverlaysEnabled(true));
    expect(mocks.setValue).toHaveBeenCalledWith(
      {
        key: "ui.card_overlays_enabled",
        value: true,
        identity: { scope: "profile" },
      },
      expect.objectContaining({ onError: expect.any(Function) }),
    );
  });

  it("writes an overlay opt-in for a profile that has expressed no preference", async () => {
    mocks.profileId = "profile-1";
    mocks.effective = {};
    mocks.api.mockResolvedValue({ enabled: false });
    const { result } = renderHook(() => useOverlayPrefs(), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isLoading).toBe(false));

    act(() => result.current.setOverlaysEnabled(true));
    expect(mocks.setValue).toHaveBeenCalledWith(
      {
        key: "ui.card_overlays_enabled",
        value: true,
        identity: { scope: "profile" },
      },
      expect.objectContaining({ onError: expect.any(Function) }),
    );
  });

  it("refreshes the shared overlay configuration immediately after an admin save", async () => {
    const queryClient = new QueryClient({
      defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
    });
    const invalidateQueries = vi.spyOn(queryClient, "invalidateQueries");
    mocks.api.mockResolvedValue({});
    const wrapper = ({ children }: { children: ReactNode }) =>
      createElement(QueryClientProvider, { client: queryClient }, children);
    const { result } = renderHook(() => useUpdateServerSettings(), { wrapper });

    await act(async () => {
      await result.current.mutateAsync({ "defaults.card_quick_actions": "favorites" });
    });

    expect(invalidateQueries).toHaveBeenCalledWith({
      queryKey: ["settings", "overlay-config"],
    });
  });
});
