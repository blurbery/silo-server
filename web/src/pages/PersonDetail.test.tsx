import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router";
import { beforeEach, expect, it, vi } from "vitest";

import { getPerson } from "@/api/v2/people";
import type { Person } from "@/api/types";
import PersonDetail from "./PersonDetail";

const { refresh } = vi.hoisted(() => ({ refresh: vi.fn() }));

vi.mock("@/api/v2/people", () => ({ getPerson: vi.fn() }));
vi.mock("@/hooks/queries/people", () => ({
  useRefreshPerson: () => ({ mutate: refresh, isPending: false }),
  invalidatePersonItemDetails: vi.fn(),
  observePersonRefresh: vi.fn(),
}));
vi.mock("@/hooks/useAuth", () => ({ useAuth: () => ({ user: { id: 1 } }) }));
vi.mock("@/hooks/useIsActingAdmin", () => ({ useIsActingAdmin: () => true }));
vi.mock("@/hooks/useDocumentTitle", () => ({ useDocumentTitle: () => {} }));
vi.mock("@/hooks/queries/catalog", () => ({
  useCatalogWindow: () => ({ data: { totalItems: 0, pages: new Map() }, isLoading: false }),
}));
vi.mock("@/components/ItemGrid", () => ({ default: () => null }));
vi.mock("@/components/PageBack", () => ({ default: () => null }));
vi.mock("@/components/EditPersonDialog", () => ({ default: () => null }));

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getPerson).mockResolvedValue({
    id: 7,
    name: "Incomplete Person",
    bio: "",
    photo_url: "",
    birth_date: null,
  } as Person);
});

it("reads incomplete details without forcing a refresh, but preserves the explicit button", async () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const view = render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={["/person/7"]}>
        <Routes>
          <Route path="/person/:id" element={<PersonDetail />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
  try {
    await screen.findByRole("heading", { name: "Incomplete Person" });
    expect(getPerson).toHaveBeenCalled();
    expect(refresh).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Refresh now" }));
    expect(refresh).toHaveBeenCalledTimes(1);
  } finally {
    view.unmount();
    client.clear();
  }
});
