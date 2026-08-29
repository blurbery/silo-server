import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import RatingsSection from "./RatingsSection";

const mocks = vi.hoisted(() => ({
  useCommunityRatings: vi.fn(),
  mutate: vi.fn(),
}));

vi.mock("@/hooks/queries/ratings", () => ({
  useCommunityRatings: mocks.useCommunityRatings,
  useSetCommunityRatingReaction: () => ({
    mutate: mocks.mutate,
    isPending: false,
  }),
}));

vi.mock("@/hooks/useCarouselEmbla", () => ({
  useCarouselEmbla: () => ({
    emblaRef: () => {},
    canScrollPrev: false,
    canScrollNext: false,
    scrollPrev: vi.fn(),
    scrollNext: vi.fn(),
  }),
}));

describe("RatingsSection", () => {
  beforeEach(() => {
    mocks.mutate.mockReset();
    mocks.useCommunityRatings.mockReturnValue({
      data: {
        average_rating: 4.5,
        vote_count: 2,
        ratings: [
          {
            key: "rating-other",
            display_name: "Sam***",
            avatar_url: "/profile-avatars/avatar-1.svg",
            rating: 5,
            up_count: 3,
            down_count: 1,
            is_viewer: false,
          },
          {
            key: "rating-viewer",
            display_name: "Blu***",
            rating: 4,
            up_count: 1,
            down_count: 0,
            is_viewer: true,
          },
        ],
      },
    });
  });

  it("renders opaque-style cards with stars and separate reaction tallies", () => {
    render(<RatingsSection itemId="movie-1" />);

    expect(screen.getByRole("heading", { name: "Ratings" })).toBeInTheDocument();
    expect(screen.getByText("4.5 average from 2 watched profiles")).toBeInTheDocument();
    expect(screen.getByText("Sam***")).toBeInTheDocument();
    expect(screen.getByLabelText("5 out of 5 stars")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Helpful: 3" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "Not helpful: 1" })).toBeEnabled();
    expect(screen.getAllByRole("article")[0]).toHaveClass("household-rating-card");
  });

  it("toggles another profile's reaction and disables reactions on your own card", () => {
    render(<RatingsSection itemId="movie-1" />);

    fireEvent.click(screen.getByRole("button", { name: "Helpful: 3" }));
    expect(mocks.mutate).toHaveBeenCalledWith({ ratingKey: "rating-other", reaction: "up" });

    const ownHelpful = screen.getByRole("button", { name: "Helpful: 1" });
    expect(ownHelpful).toBeDisabled();
  });
});
