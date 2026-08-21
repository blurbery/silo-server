import type { ItemDetail } from "@/api/types";

interface CardWatchedBadgeProps {
  mediaType: ItemDetail["type"];
  played?: boolean;
}

export default function CardWatchedBadge({ mediaType, played }: CardWatchedBadgeProps) {
  if (!played || (mediaType !== "movie" && mediaType !== "series")) {
    return null;
  }

  return (
    <span className="text-foreground/90 border-foreground/35 ml-auto inline-flex shrink-0 items-center rounded-md border px-1.5 py-0.5 text-[9px] leading-none font-semibold tracking-[0.1em] uppercase">
      Watched
    </span>
  );
}
