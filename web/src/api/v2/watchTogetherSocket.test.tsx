import { createContext, useContext, useState, type ReactNode } from "react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, renderHook, screen, waitFor } from "@testing-library/react";
import {
  captureProfileRequestContext,
  fetchWithSession,
  setRefreshToken,
  setAccessToken,
  setProfileId,
  setProfileToken,
} from "@/api/client";
import { mintRoomSocketTicket } from "./watchTogetherSocket";
import { useWatchTogetherRoomConnection } from "@/player/hooks/useWatchTogetherRoomConnection";
const AuthUpdates = createContext(0);
vi.mock("@/hooks/useAuth", () => ({ useOptionalAuth: () => useContext(AuthUpdates) }));
vi.mock("@/lib/watchTogether", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/watchTogether")>()),
  getWatchTogetherRoom: vi.fn(async (roomId: string) => ({
    room: { room_id: roomId, generation: 1 },
    room_access_token: "room-proof",
  })),
  listWatchTogetherSuggestions: vi.fn(async () => ({ suggestions: [] })),
}));
class RoomSocket extends EventTarget {
  static CONNECTING = 0;
  static OPEN = 1;
  static CLOSED = 3;
  static all: RoomSocket[] = [];
  readyState = 0;
  protocol = "silo.room.v2";
  send = vi.fn();
  constructor(
    readonly url: string,
    readonly protocols: string[],
  ) {
    super();
    RoomSocket.all.push(this);
  }
  open() {
    this.readyState = 1;
    this.dispatchEvent(new Event("open"));
  }
  close() {
    this.readyState = 3;
    this.dispatchEvent(new Event("close"));
  }
  message(data: unknown) {
    this.dispatchEvent(new MessageEvent("message", { data: JSON.stringify(data) }));
  }
}
const ticket = (letter = "a") =>
  new Response(
    JSON.stringify({
      ticket: letter.repeat(43),
      protocol: "silo.room.v2",
      expires_in: 29,
      max_connection_seconds: 300,
    }),
    { headers: { "Content-Type": "application/json" } },
  );
beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
  setAccessToken("login");
  setProfileId("profile");
  setProfileToken("pin-A");
  RoomSocket.all = [];
  vi.stubGlobal("WebSocket", RoomSocket);
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
it("captures original proof and authority without authentication replay", async () => {
  const fetch = vi.fn().mockImplementation(async () => ticket());
  vi.stubGlobal("fetch", fetch);
  await mintRoomSocketTicket("room", "original-proof");
  expect(fetch).toHaveBeenCalledTimes(1);
  expect(fetch.mock.calls[0]![0]).toBe("/api/v2/watch-together/rooms/room/ws-ticket");
  const headers = new Headers(fetch.mock.calls[0]![1].headers);
  expect(headers.get("X-Room-Token")).toBe("original-proof");
  expect(headers.get("X-Profile-Token")).toBe("pin-A");
});
it.each([401, 403, 409, 422, 500])("single sends ticket refusal %s", async (status) => {
  const fetch = vi.fn().mockResolvedValue(new Response(null, { status }));
  vi.stubGlobal("fetch", fetch);
  await expect(mintRoomSocketTicket("room", "proof")).rejects.toThrow();
  expect(fetch).toHaveBeenCalledTimes(1);
});
it("refuses replaced authority before dispatch", async () => {
  const original = captureProfileRequestContext();
  setProfileToken("pin-B");
  const fetch = vi.fn();
  vi.stubGlobal("fetch", fetch);
  await expect(mintRoomSocketTicket("room", "proof", original)).rejects.toThrow();
  expect(fetch).not.toHaveBeenCalled();
});
it("mounted socket uses no URL credentials and rebinds replaced same-profile PIN", async () => {
  const fetch = vi.fn().mockImplementation(async () => ticket());
  vi.stubGlobal("fetch", fetch);
  const view = renderHook(() =>
    useWatchTogetherRoomConnection({ roomId: "room", roomToken: "room-proof" }),
  );
  await waitFor(() => expect(RoomSocket.all.length).toBe(1));
  const old = RoomSocket.all[0]!;
  expect(new URL(old.url).pathname).toBe("/api/v2/watch-together/rooms/room/ws");
  expect(new URL(old.url).search).toBe("");
  expect(old.protocols).toEqual(["silo.room.v2", `silo.ticket.${"a".repeat(43)}`]);
  act(() => old.open());
  expect(view.result.current.connectionState).toBe("connected");
  act(() => old.message({ type: "snapshot", room: { room_id: "room", generation: 20 } }));
  expect(view.result.current.room?.generation).toBe(20);
  setProfileToken("pin-B");
  view.rerender();
  await waitFor(() => expect(RoomSocket.all.length).toBe(2));
  expect(old.readyState).toBe(RoomSocket.CLOSED);
  act(() => old.message({ type: "snapshot", room: { room_id: "room", generation: 99 } }));
  expect(view.result.current.room?.generation).not.toBe(99);
  act(() => old.close());
  expect(RoomSocket.all.length).toBe(2);
  const current = RoomSocket.all[1]!;
  act(() => current.open());
  const headers = new Headers(fetch.mock.calls[1]![1].headers);
  expect(headers.get("X-Profile-Token")).toBe("pin-B");
  act(() => {
    expect(view.result.current.sendRoomMessage({ type: "ready", session_id: "session" }).ok).toBe(
      true,
    );
  });
  setProfileToken("pin-C");
  expect(view.result.current.sendRoomMessage({ type: "ready", session_id: "session" }).ok).toBe(
    false,
  );
});
it.each([201, 500])(
  "ignores old pending ticket after authority replacement (%s)",
  async (status) => {
    const pending: Array<(r: Response) => void> = [];
    const fetch = vi.fn().mockImplementation(() => new Promise<Response>((r) => pending.push(r)));
    vi.stubGlobal("fetch", fetch);
    const view = renderHook(() =>
      useWatchTogetherRoomConnection({ roomId: "room", roomToken: "room-proof" }),
    );
    await waitFor(() => expect(pending.length).toBe(1));
    setProfileToken("pin-B");
    view.rerender();
    await waitFor(() => expect(pending.length).toBe(2));
    await act(async () =>
      pending[0]!(status === 201 ? ticket("a") : new Response(null, { status })),
    );
    expect(RoomSocket.all.length).toBe(0);
    expect(view.result.current.closedReason).toBeNull();
    await act(async () => pending[1]!(ticket("b")));
    expect(RoomSocket.all.length).toBe(1);
    expect(RoomSocket.all[0]!.protocols[1]).toBe(`silo.ticket.${"b".repeat(43)}`);
  },
);
it("gets a fresh ticket after socket close and preserves ping/message callbacks", async () => {
  let count = 0;
  const fetch = vi.fn().mockImplementation(async () => ticket(++count === 1 ? "a" : "b"));
  vi.stubGlobal("fetch", fetch);
  const view = renderHook(() =>
    useWatchTogetherRoomConnection({ roomId: "room", roomToken: "room-proof" }),
  );
  await waitFor(() => expect(RoomSocket.all.length).toBe(1));
  const old = RoomSocket.all[0]!;
  act(() => old.open());
  expect(JSON.parse(old.send.mock.calls[0]![0]).type).toBe("ping");
  act(() => old.message({ type: "transport_command", command: { action: "play" } }));
  expect(view.result.current.transportCommand?.action).toBe("play");
  act(() => old.close());
  await waitFor(() => expect(RoomSocket.all.length).toBe(2));
  expect(fetch).toHaveBeenCalledTimes(2);
  expect(RoomSocket.all[1]!.protocols[1]).toBe(`silo.ticket.${"b".repeat(43)}`);
});
it("terminal ticket refusal opens no socket and does not reconnect", async () => {
  const fetch = vi.fn().mockResolvedValue(
    new Response(
      JSON.stringify({
        type: "https://siloserver.org/docs/api/v2/problems/permission_denied",
        title: "Permission denied",
        status: 403,
      }),
      { status: 403, headers: { "Content-Type": "application/problem+json" } },
    ),
  );
  vi.stubGlobal("fetch", fetch);
  const view = renderHook(() =>
    useWatchTogetherRoomConnection({ roomId: "room", roomToken: "room-proof" }),
  );
  await waitFor(() => expect(view.result.current.closedReason).toBe("forbidden"));
  expect(RoomSocket.all.length).toBe(0);
  expect(fetch).toHaveBeenCalledTimes(1);
});

it("auth provider update reconnects without changing room props or rerendering the hook explicitly", async () => {
  function Provider({ children }: { children: ReactNode }) {
    const [revision, setRevision] = useState(0);
    return (
      <AuthUpdates.Provider value={revision}>
        <button
          onClick={() => {
            setProfileToken("provider-pin-B");
            setRevision((value) => value + 1);
          }}
        >
          Replace authority
        </button>
        {children}
      </AuthUpdates.Provider>
    );
  }
  const fetch = vi.fn().mockImplementation(async () => ticket());
  vi.stubGlobal("fetch", fetch);
  renderHook(() => useWatchTogetherRoomConnection({ roomId: "room", roomToken: "room-proof" }), {
    wrapper: Provider,
  });
  await waitFor(() => expect(RoomSocket.all.length).toBe(1));
  const old = RoomSocket.all[0]!;
  act(() => old.open());
  fireEvent.click(screen.getByRole("button", { name: "Replace authority" }));
  await waitFor(() => expect(RoomSocket.all.length).toBe(2));
  expect(old.readyState).toBe(RoomSocket.CLOSED);
  expect(new Headers(fetch.mock.calls[1]![1].headers).get("X-Profile-Token")).toBe(
    "provider-pin-B",
  );
});

it("fresh reconnect delegates an already-rotated access token under the same captured authority", async () => {
  const original = captureProfileRequestContext();
  setRefreshToken("synthetic-refresh");
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(new Response(null, { status: 401 }))
    .mockResolvedValueOnce(
      new Response(
        JSON.stringify({ access_token: "rotated-login", refresh_token: "next-refresh" }),
        { headers: { "Content-Type": "application/json" } },
      ),
    )
    .mockResolvedValueOnce(new Response(null, { status: 200 }))
    .mockResolvedValueOnce(ticket());
  vi.stubGlobal("fetch", fetch);
  await fetchWithSession("/synthetic-existing-read", {});
  await mintRoomSocketTicket("room", "original-room-proof", original);
  expect(fetch).toHaveBeenCalledTimes(4);
  expect(fetch.mock.calls[3]![0]).toBe("/api/v2/watch-together/rooms/room/ws-ticket");
  const headers = new Headers(fetch.mock.calls[3]![1].headers);
  expect(headers.get("Authorization")).toBe("Bearer rotated-login");
  expect(headers.get("X-Room-Token")).toBe("original-room-proof");
  expect(headers.get("X-Profile-Token")).toBe("pin-A");
});
