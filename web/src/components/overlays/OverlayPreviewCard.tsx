import CardOverlays from "./CardOverlays";
import { SAMPLE_MOVIE_DATA, SAMPLE_SHOW_DATA, type CardOverlayPrefs } from "@/lib/overlays";
import CardWatchedBadge from "@/components/CardWatchedBadge";
import type { WebWatchedIndicatorStyle } from "@/lib/watchedIndicator";

interface OverlayPreviewCardProps {
  prefs: CardOverlayPrefs;
  variant?: "movie" | "show";
  size?: "sm" | "md";
  showPosterOverlays?: boolean;
  watchedIndicatorStyle?: WebWatchedIndicatorStyle;
}

const SIZE_CLASSES: Record<NonNullable<OverlayPreviewCardProps["size"]>, string> = {
  sm: "w-[140px]",
  md: "w-[180px]",
};

// Shared preview component used by both the user-facing card overlays
// settings page and the admin defaults editor. Renders a 2:3 poster
// placeholder with the actual <CardOverlays /> renderer on top of it,
// fed sample data.
export function OverlayPreviewCard({
  prefs,
  variant = "movie",
  size = "md",
  showPosterOverlays = true,
  watchedIndicatorStyle,
}: OverlayPreviewCardProps) {
  const data = variant === "show" ? SAMPLE_SHOW_DATA : SAMPLE_MOVIE_DATA;
  const sizeClass = SIZE_CLASSES[size];
  const showWatchedPreview = watchedIndicatorStyle !== undefined;

  return (
    <div className={`mx-auto ${sizeClass}`}>
      <div className="bg-muted/40 relative aspect-[2/3] overflow-hidden rounded-xl border">
        <div className="text-muted-foreground/30 flex h-full items-center justify-center text-xs font-medium tracking-wider uppercase">
          {variant === "show" ? "Show preview" : "Movie preview"}
        </div>
        {showPosterOverlays ? <CardOverlays data={data} prefs={prefs} /> : null}
      </div>
      {showWatchedPreview ? (
        <div className="px-1 pt-3">
          <div className="truncate text-[14px] font-semibold tracking-tight">
            {variant === "show" ? "Example Show" : "Example Movie"}
          </div>
          <div className="text-muted-foreground mt-1 flex min-w-0 items-center gap-2 text-[11px] font-medium tracking-[0.14em] uppercase">
            <span className="min-w-0 truncate">{variant === "show" ? "2024 Series" : "2024"}</span>
            <CardWatchedBadge
              mediaType={variant === "show" ? "series" : "movie"}
              played
              style={watchedIndicatorStyle}
            />
          </div>
        </div>
      ) : null}
    </div>
  );
}
