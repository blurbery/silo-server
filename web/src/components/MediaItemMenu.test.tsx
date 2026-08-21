import { renderToStaticMarkup } from "react-dom/server";
import type { ItemDetail } from "@/api/types";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { buildMediaItemMenuModel, MetadataActionDialogHost } from "./MediaItemMenu";
import { mediaItemMenuTriggerClassName } from "./mediaItemMenuTrigger";

const mocks = vi.hoisted(() => ({
  useCatalogItemDetail: vi.fn(),
  editItem: vi.fn(),
  matchItem: vi.fn(),
}));

vi.mock("@/hooks/queries/catalogRead", () => ({
  useCatalogItemDetail: (...args: unknown[]) => mocks.useCatalogItemDetail(...args),
}));

vi.mock("@/components/EditMetadataDialog", () => ({
  default: ({ item }: { item: ItemDetail }) => {
    mocks.editItem(item);
    return <div>Edit {item.title}</div>;
  },
}));

vi.mock("@/components/MatchItemDialog", () => ({
  default: ({ item }: { item: ItemDetail }) => {
    mocks.matchItem(item);
    return <div>Match {item.title}</div>;
  },
}));

beforeEach(() => {
  mocks.useCatalogItemDetail.mockReset();
  mocks.editItem.mockReset();
  mocks.matchItem.mockReset();
});

describe("buildMediaItemMenuModel", () => {
  it("returns watched/favorite/watchlist removal labels for active state", () => {
    const model = buildMediaItemMenuModel({
      mediaType: "movie",
      userState: {
        played: true,
        is_favorite: true,
        in_watchlist: true,
      },
      isAdmin: true,
    });
    const actions = model.filter((item) => item.kind === "action");

    expect(actions[0]?.label).toBe("Play from Beginning");
    expect(actions[1]?.label).toBe("Mark Unwatched");
    expect(actions[2]?.label).toBe("Remove from Favorites");
    expect(actions[3]?.label).toBe("Remove from Watchlist");
    expect(actions[4]?.label).toBe("View Play History");
    expect(actions[5]?.label).toBe("Refresh Metadata");
    expect(actions[6]?.label).toBe("Edit Metadata");
    expect(actions[7]?.label).toBe("Match Item");
    expect(model.some((item) => item.kind === "action" && item.label === "View Play History")).toBe(
      true,
    );
    expect(model.some((item) => item.kind === "action" && item.label === "Refresh Metadata")).toBe(
      true,
    );
  });

  it("omits favorites and watchlist when showCollectionActions is false", () => {
    const model = buildMediaItemMenuModel({
      mediaType: "episode",
      userState: {
        played: false,
        is_favorite: false,
        in_watchlist: false,
      },
      isAdmin: false,
      showCollectionActions: false,
    });
    const actions = model.filter((item) => item.kind === "action");

    expect(actions).toHaveLength(1);
    expect(actions[0]?.label).toBe("Mark Watched");
  });

  it("shows watched toggle and admin actions when showCollectionActions is false for admins", () => {
    const model = buildMediaItemMenuModel({
      mediaType: "movie",
      userState: {
        played: true,
        is_favorite: false,
        in_watchlist: false,
      },
      isAdmin: true,
      showCollectionActions: false,
    });
    const actions = model.filter((item) => item.kind === "action");

    expect(actions).toHaveLength(6);
    expect(actions[0]?.label).toBe("Play from Beginning");
    expect(actions[1]?.label).toBe("Mark Unwatched");
    expect(actions[2]?.label).toBe("View Play History");
    expect(actions[3]?.label).toBe("Refresh Metadata");
    expect(actions[4]?.label).toBe("Edit Metadata");
    expect(actions[5]?.label).toBe("Match Item");
  });

  it("omits admin actions for non-admin users", () => {
    const model = buildMediaItemMenuModel({
      mediaType: "episode",
      userState: {
        played: false,
        is_favorite: false,
        in_watchlist: false,
      },
      isAdmin: false,
    });
    const actions = model.filter((item) => item.kind === "action");

    expect(actions[0]?.label).toBe("Mark Watched");
    expect(actions[1]?.label).toBe("Add to Favorites");
    expect(actions[2]?.label).toBe("Add to Watchlist");
    expect(model.some((item) => item.kind === "action" && item.label === "View Play History")).toBe(
      false,
    );
    expect(model.some((item) => item.kind === "action" && item.label === "Refresh Metadata")).toBe(
      false,
    );
  });

  it("shows metadata actions to a metadata curator without exposing play history", () => {
    const model = buildMediaItemMenuModel({
      mediaType: "series",
      isAdmin: false,
      canCurateMetadata: true,
    });
    const labels = model.filter((item) => item.kind === "action").map((item) => item.label);

    expect(labels).toEqual(["Refresh Metadata", "Edit Metadata", "Match Item"]);
    expect(labels).not.toContain("View Play History");
  });

  it("limits edit and match card actions to movies and series", () => {
    const model = buildMediaItemMenuModel({
      mediaType: "episode",
      isAdmin: true,
    });
    const labels = model.filter((item) => item.kind === "action").map((item) => item.label);

    expect(labels).toContain("Refresh Metadata");
    expect(labels).not.toContain("Edit Metadata");
    expect(labels).not.toContain("Match Item");
  });

  it("shows a continue watching dismissal action when provided", () => {
    const model = buildMediaItemMenuModel({
      mediaType: "episode",
      userState: {
        played: false,
        is_favorite: false,
        in_watchlist: false,
      },
      isAdmin: false,
      dismissLabel: "Remove from Continue Watching",
    });

    expect(
      model.some(
        (item) => item.kind === "action" && item.label === "Remove from Continue Watching",
      ),
    ).toBe(true);
  });

  it("shows a next up dismissal action when provided", () => {
    const model = buildMediaItemMenuModel({
      mediaType: "episode",
      userState: {
        played: false,
        is_favorite: false,
        in_watchlist: false,
      },
      isAdmin: false,
      dismissLabel: "Remove from Next Up",
    });

    expect(
      model.some((item) => item.kind === "action" && item.label === "Remove from Next Up"),
    ).toBe(true);
  });

  it("shows play from beginning for partially watched leaf items", () => {
    const model = buildMediaItemMenuModel({
      mediaType: "episode",
      hasPartialProgress: true,
      userState: {
        played: false,
        is_favorite: false,
        in_watchlist: false,
      },
      isAdmin: false,
      showCollectionActions: false,
    });

    expect(
      model.some((item) => item.kind === "action" && item.label === "Play from Beginning"),
    ).toBe(true);
  });

  it("uses listening labels for audiobook state actions", () => {
    const model = buildMediaItemMenuModel({
      mediaType: "audiobook",
      hasPartialProgress: true,
      userState: {
        played: false,
        is_favorite: false,
        in_watchlist: false,
      },
      isAdmin: false,
      dismissLabel: "Remove from Continue Listening",
    });
    const actions = model.filter((item) => item.kind === "action");

    expect(actions[0]?.label).toBe("Listen from Beginning");
    expect(actions[1]?.label).toBe("Mark Listened");
    expect(actions.some((item) => item.label === "Remove from Continue Listening")).toBe(true);
  });

  it("does not show play from beginning for non-leaf items", () => {
    const model = buildMediaItemMenuModel({
      mediaType: "series",
      userState: {
        played: true,
        is_favorite: false,
        in_watchlist: false,
      },
      isAdmin: false,
    });

    expect(
      model.some((item) => item.kind === "action" && item.label === "Play from Beginning"),
    ).toBe(false);
  });

  it("uses reading labels for ebook state actions", () => {
    const model = buildMediaItemMenuModel({
      mediaType: "ebook",
      userState: {
        played: false,
        is_favorite: false,
        in_watchlist: false,
      },
      isAdmin: false,
      dismissLabel: "Remove from Continue Reading",
    });
    const labels = model.filter((item) => item.kind === "action").map((item) => item.label);

    expect(labels).toEqual([
      "Mark Read",
      "Add to Favorites",
      "Add to Watchlist",
      "Remove from Continue Reading",
    ]);
    expect(labels).not.toContain("Mark Watched");
  });

  it("uses the unread label for ebooks already marked read", () => {
    const model = buildMediaItemMenuModel({
      mediaType: "ebook",
      userState: {
        played: true,
        is_favorite: false,
        in_watchlist: false,
      },
      isAdmin: false,
    });
    const labels = model.filter((item) => item.kind === "action").map((item) => item.label);

    expect(labels).toContain("Mark Unread");
  });
});

describe("MediaItemMenu metadata dialogs", () => {
  const detail = {
    content_id: "series-1",
    type: "series",
    title: "Silo",
    year: 2023,
  } as ItemDetail;

  it("loads the exact item detail and passes it to Edit Metadata", () => {
    mocks.useCatalogItemDetail.mockReturnValue({ data: detail });

    const markup = renderToStaticMarkup(
      <MetadataActionDialogHost
        action="edit"
        contentId="series-1"
        libraryId={12}
        onClose={() => undefined}
      />,
    );

    expect(mocks.useCatalogItemDetail).toHaveBeenCalledWith("series-1", 12);
    expect(mocks.editItem).toHaveBeenCalledWith(detail);
    expect(markup).toContain("Edit Silo");
  });

  it("passes the full item and library context to Match Item", () => {
    mocks.useCatalogItemDetail.mockReturnValue({ data: detail });

    const markup = renderToStaticMarkup(
      <MetadataActionDialogHost
        action="match"
        contentId="series-1"
        libraryId={12}
        onClose={() => undefined}
      />,
    );

    expect(mocks.useCatalogItemDetail).toHaveBeenCalledWith("series-1", 12);
    expect(mocks.matchItem).toHaveBeenCalledWith({ ...detail, library_id: 12 });
    expect(markup).toContain("Match Silo");
  });
});

describe("MediaItemMenu trigger visibility", () => {
  it("uses open state and keyboard focus without keeping a mouse-closed card focused", () => {
    const className = mediaItemMenuTriggerClassName();

    expect(className).toContain("md:group-hover/card:opacity-100");
    expect(className).toContain("md:data-[state=open]:opacity-100");
    expect(className).toContain("md:focus-visible:opacity-100");
    expect(className).not.toContain("group-focus-within");
  });
});
