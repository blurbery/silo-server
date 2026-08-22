import { cn } from "@/lib/utils";

export type PosterActionDensity = "standard" | "compact" | "narrow";

export function mediaItemMenuIconClassName(
  variant: "poster" | "wide" = "poster",
  density: PosterActionDensity = "standard",
) {
  return variant === "wide"
    ? "size-5"
    : density === "narrow"
      ? "size-3"
      : density === "compact"
        ? "size-3 sm:size-3.5"
        : "size-3 sm:size-4";
}

export function mediaItemMenuTriggerClassName(
  variant: "poster" | "wide" = "poster",
  density: PosterActionDensity = "standard",
) {
  return cn(
    "media-card-action-trigger inline-flex items-center justify-center rounded-md border border-border/20 bg-background/60 text-foreground shadow-sm backdrop-blur-sm transition-[opacity,background-color,color] duration-150 hover:bg-background/80 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/70",
    variant === "wide"
      ? "size-9"
      : density === "narrow"
        ? "size-6"
        : density === "compact"
          ? "size-6 sm:size-7"
          : "size-6 sm:size-8",
  );
}
