import type { ItemDetail } from "@/api/types";
import { CircleCheck, Eye } from "lucide-react";
import {
  DEFAULT_WEB_WATCHED_INDICATOR_STYLE,
  type WebWatchedIndicatorStyle,
} from "@/lib/watchedIndicator";

interface CardWatchedBadgeProps {
  mediaType: ItemDetail["type"];
  played?: boolean;
  style?: WebWatchedIndicatorStyle | null;
}

const STYLE_CLASSES: Record<WebWatchedIndicatorStyle, string> = {
  pill: "rounded-full border border-foreground/20 px-2 py-0.5",
  square: "rounded-none border border-foreground/20 px-2 py-0.5",
  text: "",
  eye: "gap-1.5",
  check: "gap-1.5",
  none: "",
};

export default function CardWatchedBadge({ mediaType, played, style }: CardWatchedBadgeProps) {
  const resolvedStyle = style === undefined ? DEFAULT_WEB_WATCHED_INDICATOR_STYLE : style;
  if (
    !played ||
    (mediaType !== "movie" && mediaType !== "series") ||
    resolvedStyle === null ||
    resolvedStyle === "none"
  ) {
    return null;
  }

  return (
    <span
      data-watched-indicator={resolvedStyle}
      className={`text-muted-foreground ml-auto inline-flex shrink-0 items-center text-[11px] leading-none font-medium tracking-[0.14em] uppercase ${STYLE_CLASSES[resolvedStyle]}`}
    >
      Watched
      {resolvedStyle === "eye" ? <Eye aria-hidden="true" className="size-3.5" /> : null}
      {resolvedStyle === "check" ? <CircleCheck aria-hidden="true" className="size-3.5" /> : null}
    </span>
  );
}
