import type { ItemDetail } from "@/api/types";
import { CircleCheck, Eye } from "lucide-react";
import {
  DEFAULT_WEB_WATCHED_INDICATOR_STYLE,
  type WebWatchedIndicatorStyle,
} from "@/lib/watchedIndicator";
import { cn } from "@/lib/utils";

interface CardWatchedBadgeProps {
  mediaType: ItemDetail["type"];
  played?: boolean;
  style?: WebWatchedIndicatorStyle | null;
  iconOnly?: boolean;
}

const STYLE_CLASSES: Record<WebWatchedIndicatorStyle, string> = {
  pill: "rounded-full border border-foreground/20 px-2 py-0.5",
  square: "rounded-none border border-foreground/20 px-2 py-0.5",
  text: "",
  eye: "gap-1.5",
  check: "gap-1.5",
  none: "",
};

export function WatchedCheckIndicator({ className }: { className?: string }) {
  return (
    <span
      role="img"
      aria-label="Watched"
      data-watched-indicator="icon-only"
      className={cn("text-muted-foreground inline-flex shrink-0 items-center", className)}
    >
      <CircleCheck aria-hidden="true" className="size-4" />
    </span>
  );
}

export default function CardWatchedBadge({
  mediaType,
  played,
  style,
  iconOnly = false,
}: CardWatchedBadgeProps) {
  const resolvedStyle = style === undefined ? DEFAULT_WEB_WATCHED_INDICATOR_STYLE : style;
  if (
    !played ||
    (mediaType !== "movie" && mediaType !== "series") ||
    resolvedStyle === null ||
    resolvedStyle === "none"
  ) {
    return null;
  }

  if (iconOnly) {
    return <WatchedCheckIndicator className="ml-auto" />;
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
