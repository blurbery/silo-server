import type { AdminUser } from "@/api/types";
import { V2ProblemError } from "@/api/v2/request";
import { setAccessToken, setProfileId, setProfileToken } from "@/api/client";
// @vitest-environment jsdom

import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import AdminUsers from "./AdminUsers";

// The page is exercised for its tab wiring only; the user table's data and the
// two invite tabs each own their own queries and tests.
vi.mock("@/hooks/useAuth", () => ({ useAuth: () => ({}) }));
vi.mock("@/hooks/queries/admin/users", () => ({
  useAdminUserCapabilities: () => ({ data: { available: true, default_profile: true } }),
  useAdminUsers: () => ({ data: mocks.users, isLoading: false }),
  useCreateUser: () => ({ mutate: vi.fn(), isPending: false }),
  useUpdateUser: () => ({ mutateAsync: mocks.update, isPending: false }),
  useDeleteUser: () => ({ mutate: vi.fn(), isPending: false }),
}));

const mocks = vi.hoisted(() => ({
  useAdminServerSettings: vi.fn(),
  users: [] as AdminUser[],
  update: vi.fn(),
  reads: 0,
}));

vi.mock("@/api/v2/adminUsers", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/api/v2/adminUsers")>()),
  getAdminUser: async () => ({
    user: { ...mocks.users[0], username: "Canonical" },
    etag: `"read-${++mocks.reads}"`,
    profileContext: (await import("@/api/client")).captureProfileRequestContext()!,
  }),
}));

vi.mock("@/hooks/queries/admin/settings", () => ({
  useAdminServerSettings: (...args: unknown[]) => mocks.useAdminServerSettings(...args),
}));

vi.mock("@/hooks/queries/admin/libraries", () => ({
  useAdminLibraries: () => ({ data: [] }),
}));

vi.mock("@/hooks/queries/admin/accessGroups", () => ({
  useAccessGroups: () => ({ data: [] }),
}));

vi.mock("./admin-settings/InvitationsTab", () => ({
  default: () => <div>Invitations panel</div>,
}));

vi.mock("./admin-settings/InviteCodesTab", () => ({
  default: () => <div>Invite codes panel</div>,
}));

function LocationProbe() {
  const location = useLocation();
  return <span data-testid="location">{`${location.pathname}${location.search}`}</span>;
}

function renderPage(entry = "/admin/users") {
  return render(
    <MemoryRouter initialEntries={[entry]}>
      <Routes>
        <Route
          path="/admin/users"
          element={
            <>
              <AdminUsers />
              <LocationProbe />
            </>
          }
        />
      </Routes>
    </MemoryRouter>,
  );
}

function tab(name: string) {
  return screen.getByRole("tab", { name });
}

describe("AdminUsers tabs", () => {
  beforeEach(() => {
    setAccessToken("account");
    setProfileId("owner");
    setProfileToken(null);
    mocks.users = [];
    mocks.reads = 0;
    mocks.update.mockReset();
    mocks.useAdminServerSettings.mockReset();
    mocks.useAdminServerSettings.mockReturnValue({
      data: { "signup.enabled": "false" },
      isLoading: false,
    });
  });

  it("opens on the users tab when no tab is requested", () => {
    renderPage();

    expect(tab("Users")).toHaveAttribute("aria-selected", "true");
    expect(tab("Invite Codes")).toHaveAttribute("aria-selected", "false");
  });

  it("selects the Invite Codes tab from ?tab=invite-codes", () => {
    renderPage("/admin/users?tab=invite-codes");

    expect(tab("Invite Codes")).toHaveAttribute("aria-selected", "true");
    expect(screen.getByText("Invite codes panel")).toBeInTheDocument();
  });

  it("falls back to the users tab for an unknown tab id", () => {
    renderPage("/admin/users?tab=not-a-tab");

    expect(tab("Users")).toHaveAttribute("aria-selected", "true");
  });

  it("writes the selected tab to the URL and drops the param on the default tab", async () => {
    renderPage();

    await userEvent.click(tab("Invitations"));
    expect(screen.getByTestId("location")).toHaveTextContent("/admin/users?tab=invitations");

    await userEvent.click(tab("Users"));
    expect(screen.getByTestId("location")).toHaveTextContent("/admin/users");
    expect(screen.getByTestId("location")).not.toHaveTextContent("tab=");
  });
});

describe("AdminUsers public-signup status badge", () => {
  beforeEach(() => {
    setAccessToken("account");
    setProfileId("owner");
    setProfileToken(null);
    mocks.users = [];
    mocks.reads = 0;
    mocks.update.mockReset();
    mocks.useAdminServerSettings.mockReset();
  });

  it("shows a neutral 'off' badge linking to General settings when signups are disabled", () => {
    mocks.useAdminServerSettings.mockReturnValue({
      data: { "signup.enabled": "false" },
      isLoading: false,
    });
    renderPage();

    const badge = screen.getByText("Public signups off");
    expect(badge).toHaveAttribute("data-variant", "secondary");
    const link = badge.closest("a");
    expect(link).toHaveAttribute("href", "/admin/settings/general");
  });

  it("shows a positive 'on' badge linking to General settings when signups are enabled", () => {
    mocks.useAdminServerSettings.mockReturnValue({
      data: { "signup.enabled": "true" },
      isLoading: false,
    });
    renderPage();

    const badge = screen.getByText("Public signups on");
    expect(badge).toHaveAttribute("data-variant", "outline");
    const link = badge.closest("a");
    expect(link).toHaveAttribute("href", "/admin/settings/general");
  });

  it("renders no signup-status badge while settings are still loading", () => {
    mocks.useAdminServerSettings.mockReturnValue({ data: undefined, isLoading: true });
    renderPage();

    expect(screen.queryByText("Public signups on")).not.toBeInTheDocument();
    expect(screen.queryByText("Public signups off")).not.toBeInTheDocument();
  });

  it("stays visible regardless of which tab is active", async () => {
    mocks.useAdminServerSettings.mockReturnValue({
      data: { "signup.enabled": "true" },
      isLoading: false,
    });
    renderPage();

    expect(screen.getByText("Public signups on")).toBeInTheDocument();

    await userEvent.click(tab("Invite Codes"));
    expect(screen.getByText("Public signups on")).toBeInTheDocument();

    await userEvent.click(tab("Invitations"));
    expect(screen.getByText("Public signups on")).toBeInTheDocument();
  });
});

const adminUser: AdminUser = {
  id: 7,
  username: "taylor",
  email: "taylor@example.test",
  role: "user",
  permissions: [],
  enabled: true,
  library_ids: null,
  access_group_id: null,
  max_playback_quality: null,
  max_streams: null,
  max_transcodes: null,
  transcode_allowed: null,
  audio_transcode_allowed: null,
  max_profiles: 4,
  download_allowed: null,
  download_transcode_allowed: null,
  requests_allowed: null,
  effective_policy: {
    library_ids: null,
    max_playback_quality: "",
    max_streams: 0,
    max_transcodes: 0,
    transcode_allowed: true,
    audio_transcode_allowed: true,
    download_allowed: true,
    download_transcode_allowed: true,
    requests_allowed: true,
    permissions: [],
  },
  created_at: "2026-07-01T12:00:00Z",
  updated_at: "2026-07-01T12:00:00Z",
};

it("seeds list edits from canonical GET and preserves drafts through explicit conflict reload", async () => {
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
  setAccessToken("account");
  setProfileId("owner");
  setProfileToken(null);
  mocks.users = [adminUser];
  mocks.reads = 0;
  mocks.update
    .mockRejectedValueOnce(
      new V2ProblemError("updateAdminUser", {
        type: "https://silo.example/problems/precondition_failed",
        title: "Changed",
        status: 412,
        detail: "Reload",
        instance: "/api/v2/admin/users/7",
      }),
    )
    .mockResolvedValue(undefined);
  const user = userEvent.setup();
  renderPage();
  await user.click(screen.getByRole("button", { name: "Edit taylor" }));
  const dialog = await screen.findByRole("dialog");
  const name = within(dialog).getByLabelText("Username");
  expect(name).toHaveValue("Canonical");
  await user.clear(name);
  await user.type(name, "My draft");
  const save = within(dialog).getByRole("button", { name: /save/i });
  await user.click(save);
  await screen.findByText(/Your draft is preserved/);
  expect(name).toHaveValue("My draft");
  expect(save).toBeDisabled();
  expect(mocks.reads).toBe(1);
  await user.click(within(dialog).getByRole("button", { name: "Reload current user" }));
  await waitFor(() => expect(save).toBeEnabled());
  expect(name).toHaveValue("My draft");
  await user.click(save);
  await waitFor(() => expect(mocks.update).toHaveBeenCalledTimes(2));
  expect(mocks.update.mock.calls.map((call) => call[0].editor.etag)).toEqual([
    '"read-1"',
    '"read-2"',
  ]);
});
