export const WEB_WATCHED_INDICATOR_SETTING_KEY = "ui.web_watched_indicator";

export const WEB_WATCHED_INDICATOR_STYLES = [
  "pill",
  "square",
  "text",
  "eye",
  "check",
  "none",
] as const;

export type WebWatchedIndicatorStyle = (typeof WEB_WATCHED_INDICATOR_STYLES)[number];

export const DEFAULT_WEB_WATCHED_INDICATOR_STYLE: WebWatchedIndicatorStyle = "pill";

export const WEB_WATCHED_INDICATOR_OPTIONS: ReadonlyArray<{
  value: WebWatchedIndicatorStyle;
  label: string;
}> = [
  { value: "pill", label: "Rounded pill" },
  { value: "square", label: "Square outline" },
  { value: "text", label: "Text only" },
  { value: "eye", label: "Text + eye" },
  { value: "check", label: "Text + check" },
  { value: "none", label: "None" },
];

export function parseWebWatchedIndicatorStyle(value: unknown): WebWatchedIndicatorStyle {
  return typeof value === "string" &&
    WEB_WATCHED_INDICATOR_STYLES.some((candidate) => candidate === value)
    ? (value as WebWatchedIndicatorStyle)
    : DEFAULT_WEB_WATCHED_INDICATOR_STYLE;
}
