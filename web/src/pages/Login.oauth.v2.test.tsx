// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import Login from "./Login";
vi.mock("@/hooks/useAuth", () => ({
  useAuth: () => ({
    loading: false,
    setupLoading: false,
    setupRequired: false,
    user: null,
    providers: [
      { id: "oauth-fixture", installation_id: 3, mode: "oauth", display_name: "Fixture provider" },
    ],
  }),
  getBootstrapProfile: vi.fn(),
}));
vi.mock("@/hooks/useServerBranding", () => ({
  useServerBranding: () => ({ serverName: "Silo", loginSubtitle: "Sign in" }),
}));
vi.mock("@/hooks/useDocumentTitle", () => ({ useDocumentTitle: vi.fn() }));
vi.mock("@/components/auth/AuthBackground", () => ({ AuthBackground: () => null }));
afterEach(cleanup);
it("submits provider login as a browser POST to v2 and preserves the encoded local destination", () => {
  render(
    <MemoryRouter initialEntries={["/login?redirect=%2Fme%3Ftab%3Dsettings"]}>
      <Login />
    </MemoryRouter>,
  );
  const form = screen.getByRole("button", { name: "Fixture provider" }).closest("form");
  expect(form?.getAttribute("method")).toBe("post");
  expect(form?.getAttribute("action")).toBe(
    "/api/v2/auth/oauth/3/init?next=%2Fme%3Ftab%3Dsettings",
  );
});
