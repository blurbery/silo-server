import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import CardWatchedBadge from "./CardWatchedBadge";
import {
  DEFAULT_WEB_WATCHED_INDICATOR_STYLE,
  parseWebWatchedIndicatorStyle,
  WEB_WATCHED_INDICATOR_OPTIONS,
  type WebWatchedIndicatorStyle,
} from "@/lib/watchedIndicator";

function renderBadge(style?: WebWatchedIndicatorStyle | null) {
  return renderToStaticMarkup(<CardWatchedBadge mediaType="series" played style={style} />);
}

describe("CardWatchedBadge", () => {
  it("uses the subtle rounded pill by default", () => {
    const markup = renderBadge();

    expect(markup).toContain('data-watched-indicator="pill"');
    expect(markup).toContain("rounded-full");
    expect(markup).toContain("border-foreground/20");
    expect(markup).toContain("text-muted-foreground");
    expect(markup).toContain("text-[11px]");
    expect(markup).toContain("font-medium");
  });

  it.each([
    ["square", "rounded-none"],
    ["text", 'data-watched-indicator="text"'],
    ["eye", "lucide-eye"],
    ["check", "lucide-circle-check"],
  ] as const)("renders the %s style", (style, expected) => {
    expect(renderBadge(style)).toContain(expected);
  });

  it("supports disabling the web indicator", () => {
    expect(renderBadge("none")).toBe("");
  });

  it.each(["pill", "square", "text", "eye", "check"] as const)(
    "uses the circle-check without text for the enabled %s style in icon-only mode",
    (style) => {
      const markup = renderToStaticMarkup(
        <CardWatchedBadge mediaType="series" played style={style} iconOnly />,
      );

      expect(markup).toContain('data-watched-indicator="icon-only"');
      expect(markup).toContain('role="img"');
      expect(markup).toContain('aria-label="Watched"');
      expect(markup).toContain("lucide-circle-check");
      expect(markup).not.toContain(">Watched<");
    },
  );

  it.each(["none", null] as const)(
    "still hides the icon-only indicator for the %s backend state",
    (style) => {
      expect(
        renderToStaticMarkup(<CardWatchedBadge mediaType="series" played style={style} iconOnly />),
      ).toBe("");
    },
  );

  it("stays hidden while the server-wide style is loading", () => {
    expect(renderBadge(null)).toBe("");
  });

  it("still gates display on profile watched state and root media type", () => {
    expect(
      renderToStaticMarkup(<CardWatchedBadge mediaType="movie" played={false} style="pill" />),
    ).toBe("");
    expect(renderToStaticMarkup(<CardWatchedBadge mediaType="episode" played style="pill" />)).toBe(
      "",
    );
  });
});

describe("web watched indicator settings", () => {
  it("exposes the five visual choices plus none", () => {
    expect(WEB_WATCHED_INDICATOR_OPTIONS.map((option) => option.value)).toEqual([
      "pill",
      "square",
      "text",
      "eye",
      "check",
      "none",
    ]);
  });

  it("falls back safely when the server returns an unknown choice", () => {
    expect(parseWebWatchedIndicatorStyle("unknown")).toBe(DEFAULT_WEB_WATCHED_INDICATOR_STYLE);
  });
});
