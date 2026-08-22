import type { ComponentProps } from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { describe, expect, it, vi } from "vitest";
import type { FileVersion } from "@/api/types";
import ActionBar from "./ActionBar";

vi.mock("@/playback/watchPlaybackContext", () => ({
  useWatchPlaybackController: () => ({ startPlayback: vi.fn() }),
}));

vi.mock("@/components/AddToCollectionDialog", () => ({
  default: () => null,
}));

vi.mock("./SubtitlesPopover", () => ({
  default: () => null,
}));

type ActionBarProps = ComponentProps<typeof ActionBar>;

const selectedVersion: FileVersion = {
  file_id: 1,
  resolution: "1080p",
  codec_video: "h264",
  codec_audio: "aac",
  hdr: false,
  container: "mkv",
  file_size: 0,
  duration: 7_200,
  bitrate: 0,
};

const playBranches: Array<[string, Partial<ActionBarProps>]> = [
  ["standard", {}],
  ["selected version", { selectedVersion }],
  ["resume choice", { playLabel: "Resume", restartHref: "/watch/movie-1?restart=1" }],
];

function renderActionBar(overrides: Partial<ActionBarProps> = {}) {
  return render(
    <MemoryRouter>
      <ActionBar playHref="/watch/movie-1" {...overrides} />
    </MemoryRouter>,
  );
}

describe("ActionBar", () => {
  it.each(playBranches)("keeps the %s Play action responsive", (_, overrides) => {
    renderActionBar(overrides);

    expect(screen.getByRole("button", { name: "Play" })).toHaveClass(
      "cursor-pointer",
      "transform-gpu",
      "transition-none",
    );
  });

  it("keeps the watched action responsive and shows pointers on enabled actions", () => {
    renderActionBar({
      watchedLabel: "Mark Watched",
      onToggleWatched: () => {},
      onToggleFavorite: () => {},
    });

    expect(screen.getByRole("button", { name: "Mark Watched" })).toHaveClass(
      "cursor-pointer",
      "transform-gpu",
      "transition-none",
    );
    expect(screen.getByTitle("Favorite")).toHaveClass("cursor-pointer");
    expect(screen.getByTitle("More")).toHaveClass("cursor-pointer");
  });

  it("uses matching icons and longest-entry sizing in the detail menu", async () => {
    render(
      <MemoryRouter>
        <ActionBar
          contentId="series-1"
          isAdmin
          canCurateMetadata
          onToggleWatchlist={() => {}}
          onRefresh={() => {}}
          onEditMetadata={() => {}}
          onMatchItem={() => {}}
          onSplitItem={() => {}}
        />
      </MemoryRouter>,
    );

    await userEvent.click(screen.getByTitle("More"));

    const menu = screen.getByRole("menu");
    expect(menu).toHaveClass("w-max", "max-w-[calc(100vw-2rem)]", "min-w-0");
    expect(menu).not.toHaveClass("w-56");
    for (const item of screen.getAllByRole("menuitem")) {
      expect(item.querySelector("svg"), item.textContent ?? "menu item").toBeTruthy();
    }
    expect(
      screen.getByRole("menuitem", { name: "View Play History" }).querySelector(".lucide-history"),
    ).toBeTruthy();
    expect(
      screen
        .getByRole("menuitem", { name: "Refresh Metadata" })
        .querySelector(".lucide-refresh-cw"),
    ).toBeTruthy();
  });
});
