export const CARD_QUICK_ACTION_MODES = ["both", "favorites", "watched"] as const;

export type EnabledCardQuickActionMode = (typeof CARD_QUICK_ACTION_MODES)[number];
export type CardQuickActionMode = EnabledCardQuickActionMode | "none";

export const CARD_QUICK_ACTION_OPTIONS: ReadonlyArray<{
  value: EnabledCardQuickActionMode;
  label: string;
}> = [
  { value: "both", label: "Both" },
  { value: "favorites", label: "Favorites only" },
  { value: "watched", label: "Watch indicator only" },
];

export function normalizeCardQuickActionMode(
  value: unknown,
  fallback: EnabledCardQuickActionMode = "both",
): EnabledCardQuickActionMode {
  return typeof value === "string" &&
    CARD_QUICK_ACTION_MODES.includes(value as EnabledCardQuickActionMode)
    ? (value as EnabledCardQuickActionMode)
    : fallback;
}

export function showsFavoriteQuickAction(mode: CardQuickActionMode): boolean {
  return mode === "both" || mode === "favorites";
}

export function showsWatchedQuickAction(mode: CardQuickActionMode): boolean {
  return mode === "both" || mode === "watched";
}
