// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { afterEach, expect, it, vi } from "vitest";
import HistoryImportSettings from "./HistoryImportSettings";
import { useHistoryImportRun } from "@/hooks/queries/history-import";
const state = vi.hoisted(() => ({ refresh: vi.fn() }));
vi.mock("@/components/realtimeEventsContext", () => ({ useEventChannel: vi.fn() }));
vi.mock("@/hooks/useCurrentProfile", () => ({
  useCurrentProfile: () => ({ profile: { id: "p" } }),
}));
vi.mock("@/hooks/queries/profiles", () => ({
  useProfiles: () => ({ data: [{ id: "p", name: "Member" }] }),
}));
vi.mock("@/hooks/queries/history-import", () => ({
  useHistoryImportSources: () => ({ data: [], isLoading: false }),
  useHistoryImportRuns: () => ({
    data: [
      {
        id: "saved",
        status: "canceling",
        source_type: "plex",
        connection_mode: "plex_oauth",
        terminal: false,
        cancelable: false,
        created_at: "2026-09-01T00:00:00Z",
        fetched: 0,
        matched: 0,
        unmatched: 0,
        progress_updated: 0,
        history_created: 0,
        watchlist_added: 0,
        favorites_imported: 0,
        skipped: 0,
        warnings: [],
        unmatched_samples: [],
      },
    ],
  }),
  useHistoryImportRun: vi.fn(() => ({
    data: undefined,
    error: new Error("unavailable"),
    refetch: state.refresh,
  })),
  useLoginEmbyConnect: () => ({ isPending: false }),
  useCreateHistoryImportRun: () => ({ isPending: false }),
}));
afterEach(cleanup);
it("monitors the latest persisted run and lets failed polling be refreshed", () => {
  render(
    <MemoryRouter>
      <HistoryImportSettings />
    </MemoryRouter>,
  );
  expect(useHistoryImportRun).toHaveBeenCalledWith("saved");
  expect(screen.getAllByText("Cancelling").length).toBeGreaterThan(0);
  expect(screen.getByRole("alert").textContent).toContain("Check its status");
  fireEvent.click(screen.getByRole("button", { name: "Refresh status" }));
  expect(state.refresh).toHaveBeenCalledTimes(1);
  expect(screen.queryByRole("button", { name: "Cancel import" })).toBeNull();
});
