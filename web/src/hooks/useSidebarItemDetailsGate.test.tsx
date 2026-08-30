import { act, renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { useSidebarItemDetailsGate } from "./useSidebarItemDetailsGate";

describe("useSidebarItemDetailsGate", () => {
  it("gates every non-item to item entry, including re-entering a history entry", () => {
    const { result, rerender } = renderHook(
      ({
        locationKey,
        pathname,
        isItem,
      }: {
        locationKey: string;
        pathname: string;
        isItem: boolean;
      }) => useSidebarItemDetailsGate(locationKey, pathname, isItem),
      { initialProps: { locationKey: "catalog", pathname: "/catalog", isItem: false } },
    );

    rerender({ locationKey: "movie", pathname: "/item/movie", isItem: true });
    expect(result.current.itemDetailsReady).toBe(false);

    act(() => result.current.reveal("movie"));
    expect(result.current.itemDetailsReady).toBe(true);

    rerender({ locationKey: "catalog", pathname: "/catalog", isItem: false });
    rerender({ locationKey: "movie", pathname: "/item/movie", isItem: true });
    expect(result.current.itemDetailsReady).toBe(false);
  });

  it("does not gate a direct initial item render", () => {
    const { result } = renderHook(() => useSidebarItemDetailsGate("movie", "/item/movie", true));

    expect(result.current.itemDetailsReady).toBe(true);
    expect(result.current.pendingLocationKey).toBeNull();
    expect(result.current.enteredItemFromHome).toBe(false);
  });

  it("remembers a Home item entry after its detail gate settles", () => {
    const { result, rerender } = renderHook(
      ({
        locationKey,
        pathname,
        isItem,
      }: {
        locationKey: string;
        pathname: string;
        isItem: boolean;
      }) => useSidebarItemDetailsGate(locationKey, pathname, isItem),
      { initialProps: { locationKey: "home", pathname: "/", isItem: false } },
    );

    rerender({ locationKey: "movie", pathname: "/item/movie", isItem: true });
    expect(result.current.enteredItemFromHome).toBe(true);

    rerender({ locationKey: "movie", pathname: "/item/movie", isItem: true });
    expect(result.current.enteredItemFromHome).toBe(true);

    act(() => result.current.reveal("movie"));
    expect(result.current.enteredItemFromHome).toBe(true);

    rerender({ locationKey: "home-again", pathname: "/", isItem: false });
    expect(result.current.enteredItemFromHome).toBe(false);
  });

  it("does not hold cached details for non-Home item entries", () => {
    const { result, rerender } = renderHook(
      ({
        locationKey,
        pathname,
        isItem,
      }: {
        locationKey: string;
        pathname: string;
        isItem: boolean;
      }) => useSidebarItemDetailsGate(locationKey, pathname, isItem),
      { initialProps: { locationKey: "library", pathname: "/library/1", isItem: false } },
    );

    rerender({ locationKey: "movie", pathname: "/item/movie", isItem: true });

    expect(result.current.enteredItemFromHome).toBe(false);
  });

  it("discards an abandoned gate and allows the next item entry", () => {
    const { result, rerender } = renderHook(
      ({
        locationKey,
        pathname,
        isItem,
      }: {
        locationKey: string;
        pathname: string;
        isItem: boolean;
      }) => useSidebarItemDetailsGate(locationKey, pathname, isItem),
      { initialProps: { locationKey: "catalog", pathname: "/catalog", isItem: false } },
    );

    rerender({ locationKey: "abandoned-movie", pathname: "/item/abandoned", isItem: true });
    expect(result.current.itemDetailsReady).toBe(false);

    rerender({ locationKey: "search", pathname: "/catalog", isItem: false });
    expect(result.current.itemDetailsReady).toBe(true);

    rerender({ locationKey: "next-movie", pathname: "/item/next", isItem: true });
    expect(result.current.itemDetailsReady).toBe(false);

    act(() => result.current.reveal("abandoned-movie"));
    expect(result.current.itemDetailsReady).toBe(false);

    act(() => result.current.reveal("next-movie"));
    expect(result.current.itemDetailsReady).toBe(true);
  });
});
