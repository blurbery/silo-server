import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { describe, expect, it, vi } from "vitest";
import ActionBar from "./ActionBar";

vi.mock("@/playback/watchPlaybackContext", () => ({
  useWatchPlaybackController: () => ({ startPlayback: vi.fn() }),
}));

describe("ActionBar", () => {
  it("shows the pointer cursor on its enabled primary actions", () => {
    render(
      <MemoryRouter>
        <ActionBar
          playHref="/watch/movie-1"
          watchedLabel="Mark Watched"
          onToggleWatched={() => {}}
          onToggleFavorite={() => {}}
        />
      </MemoryRouter>,
    );

    expect(screen.getByRole("button", { name: "Play" })).toHaveClass("cursor-pointer");
    expect(screen.getByRole("button", { name: "Mark Watched" })).toHaveClass("cursor-pointer");
    expect(screen.getByTitle("Favorite")).toHaveClass("cursor-pointer");
    expect(screen.getByTitle("More")).toHaveClass("cursor-pointer");
  });
});
