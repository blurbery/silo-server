import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import AdminActivity from "./AdminActivity";
const mocks = vi.hoisted(() => ({ error: false, rows: true, next: vi.fn(), restart: vi.fn() }));
vi.mock("@/hooks/queries/admin/stats", () => ({
  useAdminSessions: () => ({ data: [], isLoading: false, refetch: vi.fn() }),
}));
vi.mock("@/components/realtimeEventsContext", () => ({
  useRealtimeEvents: () => ({ connectionState: "connected" }),
}));
vi.mock("@/hooks/usePageActivity", () => ({
  usePageActivity: () => ({ canApplyRealtimeUpdates: true }),
}));
vi.mock("@/hooks/queries/admin/logs", () => ({
  useOperationalLogs: () => ({ data: [], isLoading: false }),
}));
vi.mock("@/hooks/queries/admin/ips", () => ({
  useIPUsers: () => ({
    data: mocks.rows
      ? [
          {
            user_id: 7,
            username: "Target",
            first_seen: "2026-01-01T00:00:00Z",
            last_seen: "2026-01-02T00:00:00Z",
            request_count: 3,
          },
        ]
      : [],
    isLoading: false,
    isError: mocks.error,
    hasNextPage: true,
    isFetchingNextPage: false,
    fetchNextPage: mocks.next,
    restart: mocks.restart,
  }),
}));
beforeEach(() => {
  vi.clearAllMocks();
  mocks.error = false;
  mocks.rows = true;
});
afterEach(cleanup);
function lookup() {
  render(
    <MemoryRouter>
      <AdminActivity />
    </MemoryRouter>,
  );
  fireEvent.click(screen.getByText("IP Lookup"));
  fireEvent.change(screen.getByPlaceholderText(/IP lookup/), { target: { value: "198.51.100.1" } });
  fireEvent.click(screen.getByRole("button", { name: "Lookup" }));
}
it("offers explicit continuation and preserves existing rows on partial errors", () => {
  mocks.error = true;
  lookup();
  expect(screen.getByText("Target")).toBeInTheDocument();
  expect(screen.getByText(/Could not load more history/)).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Load more" }));
  expect(mocks.next).toHaveBeenCalledTimes(1);
  fireEvent.click(screen.getByRole("button", { name: "Reload history" }));
  expect(mocks.restart).toHaveBeenCalledTimes(1);
});
it("does not present a failed initial page as an empty result", () => {
  mocks.error = true;
  mocks.rows = false;
  lookup();
  expect(screen.getByText(/Could not load IP history/)).toBeInTheDocument();
  expect(screen.queryByText(/No users found/)).not.toBeInTheDocument();
});
