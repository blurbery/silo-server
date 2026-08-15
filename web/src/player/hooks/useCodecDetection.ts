import { useEffect, useState } from "react";
import {
  detectHLSSupport,
  detectNativeHLSSupport,
  type WebCapabilityProbe,
} from "../client-context-v3";

/** Maps our codec names to the MIME declarations browsers expose for them. */
const VIDEO_CODEC_MAP: Record<string, string> = {
  h264: "avc1.640028",
  hevc: "hev1.1.6.L120.90",
  av1: "av01.0.08M.08",
  vp9: "vp09.00.10.08",
};

// Silo's native Dolby Vision remux recipe preserves the DOVI configuration
// record under a dvh1 sample entry — the one Apple's HLS authoring spec calls
// for, and the only one Safari answers "probably" for. Probe exactly that
// shape: a browser that recognizes only dvhe could accept a claim here and
// then reject the dvh1 file the remux delivers, so a dvhe-only answer earns no
// claim and that browser keeps the validated HDR10 fallback instead.
// Each profile is tested from its highest supported level down so the planner
// can enforce the browser's real bound without excluding compatible sources.
const DOLBY_VISION_PROFILE_PROBES: Record<
  number,
  { levels: number[]; blCompatibilityIds?: number[] }
> = {
  // Apple's HLS authoring specification supports Profile 5 through level 7.
  // Probe the highest level first and publish only the best exact answer.
  5: { levels: [7, 6] },
  // The MIME codec string identifies Profile 8 but not its base-layer
  // compatibility ID. Conservatively claim only the Profile 8.1 shape that
  // Safari's progressive remux path exercises. Exact level probes avoid
  // forcing a compatible high-level source down to its HDR10 fallback.
  8: { levels: [9, 8, 7, 6], blCompatibilityIds: [1] },
};

// Silo's Profile 7 fallback strips Dolby Vision metadata into a progressive
// MP4 whose video is a 2160p HEVC Main10 HDR10 base layer. Media Capabilities
// can query the codec, transfer function, gamut, and static metadata together,
// avoiding the old mistake of treating a generic HDR output query as proof of
// every HDR format.
// The strip remux labels its output hvc1 — the sample entry Apple requires and
// the one Safari answers for — so that is the only entry probed: an hev1-only
// answer is evidence for a file Silo never sends and earns no claim.
const HDR10_PROGRESSIVE_CONFIGURATION = {
  type: "file",
  video: {
    contentType: 'video/mp4; codecs="hvc1.2.4.L153.B0"',
    width: 3840,
    height: 2160,
    bitrate: 80_000_000,
    framerate: 24,
    colorGamut: "rec2020",
    transferFunction: "pq",
    hdrMetadataType: "smpteSt2086",
  },
} satisfies MediaDecodingConfiguration;

// hls.js feeds FFmpeg's default `hev1` fMP4 samples through MediaSource. Keep
// this evidence separate from the media-element `hvc1` probe above: Firefox on
// macOS can accept one route and reject the other even when a generic HEVC
// query says both are available.
const HDR10_HLS_CONFIGURATION = {
  type: "media-source",
  video: {
    contentType: 'video/mp4; codecs="hev1.2.4.L153.B0"',
    width: 3840,
    height: 2160,
    bitrate: 80_000_000,
    framerate: 24,
    colorGamut: "rec2020",
    transferFunction: "pq",
    hdrMetadataType: "smpteSt2086",
  },
} satisfies MediaDecodingConfiguration;

// Profile 8.4 has an HLG-compatible base layer. Silo removes its Dolby Vision
// metadata before fallback delivery and emits hvc1, so probe that exact 4K HLG
// shape independently from Dolby Vision and HDR10.
const HLG_PROGRESSIVE_CONFIGURATION = {
  type: "file",
  video: {
    contentType: 'video/mp4; codecs="hvc1.2.4.L153.B0"',
    width: 3840,
    height: 2160,
    bitrate: 80_000_000,
    framerate: 24,
    colorGamut: "rec2020",
    transferFunction: "hlg",
  },
} satisfies MediaDecodingConfiguration;

const HLG_HLS_CONFIGURATION = {
  ...HDR10_HLS_CONFIGURATION,
  video: {
    ...HDR10_HLS_CONFIGURATION.video,
    transferFunction: "hlg",
    hdrMetadataType: undefined,
  },
} satisfies MediaDecodingConfiguration;

const AUDIO_CODEC_MAP: Record<string, string[]> = {
  aac: ['audio/mp4; codecs="mp4a.40.2"', 'video/mp4; codecs="mp4a.40.2"'],
  mp3: ["audio/mpeg"],
  opus: [
    'audio/mp4; codecs="opus"',
    'video/mp4; codecs="opus"',
    'audio/ogg; codecs="opus"',
    'audio/webm; codecs="opus"',
  ],
  vorbis: ['audio/ogg; codecs="vorbis"', 'audio/webm; codecs="vorbis"'],
  flac: ["audio/flac", 'audio/mp4; codecs="flac"', 'video/mp4; codecs="flac"'],
  ac3: ['audio/mp4; codecs="ac-3"', 'video/mp4; codecs="ac-3"'],
  eac3: ['audio/mp4; codecs="ec-3"', 'video/mp4; codecs="ec-3"'],
  dts: ['audio/mp4; codecs="dts+"', 'video/mp4; codecs="dts+"'],
};

// The scanner normalizes M4A/M4B to mp4. Standalone MP3, FLAC, and OGG keep
// their own container keys, so they need matching direct-play probes here.
const CONTAINER_MAP: Record<string, string[]> = {
  mp4: ['video/mp4; codecs="avc1.640028"', 'audio/mp4; codecs="mp4a.40.2"'],
  webm: ['video/webm; codecs="vp09.00.10.08"'],
  mkv: ['video/x-matroska; codecs="avc1.640028"'],
  mp3: ["audio/mpeg"],
  flac: ["audio/flac"],
  ogg: ["audio/ogg"],
};

export function detectMaxResolutionFromScreen(screenWidth: number, screenHeight: number): string {
  const screenH = Math.max(screenHeight, screenWidth);
  if (screenH >= 2160) return "2160p";
  if (screenH >= 1440) return "1080p";
  if (screenH >= 720) return "720p";
  return "480p";
}

/**
 * Detects HDR display support (best effort). Firefox's `dynamic-range` query
 * reflects the browser canvas and reports `standard` even on HDR displays;
 * the video plane is exposed via `video-dynamic-range` (Firefox 116+), so
 * accept either. Browsers treat unknown media features as non-matching, so
 * querying both is safe everywhere.
 */
export function detectHDRFromMatchMedia(matchMediaFn: typeof matchMedia | undefined): boolean {
  if (!matchMediaFn) return false;
  return (
    matchMediaFn("(dynamic-range: high)").matches ||
    matchMediaFn("(video-dynamic-range: high)").matches
  );
}

/**
 * Probes the exact HDR10 progressive shape produced by Silo's remux path.
 * Deliberately independent of the `dynamic-range` media query: that query
 * describes the active output, not the decoder, and browsers tone-map HDR
 * content onto SDR outputs.
 */
export async function probeHDR10PlaybackSupport(): Promise<boolean> {
  if (typeof navigator === "undefined" || !navigator.mediaCapabilities) return false;

  try {
    const result = await navigator.mediaCapabilities.decodingInfo(HDR10_PROGRESSIVE_CONFIGURATION);
    return result.supported && result.smooth;
  } catch {
    return false;
  }
}

/** Probes the exact `hev1` HDR10 fMP4 shape consumed by hls.js. */
export async function probeHLSHDR10PlaybackSupport(): Promise<boolean> {
  if (
    typeof navigator === "undefined" ||
    !navigator.mediaCapabilities ||
    typeof MediaSource === "undefined"
  ) {
    return false;
  }
  try {
    if (!MediaSource.isTypeSupported(HDR10_HLS_CONFIGURATION.video.contentType)) return false;
    const result = await navigator.mediaCapabilities.decodingInfo(HDR10_HLS_CONFIGURATION);
    return result.supported && result.smooth;
  } catch {
    return false;
  }
}

/** Probes the exact clean HLG base-layer shape emitted for Profile 8.4. */
export async function probeHLGPlaybackSupport(): Promise<boolean> {
  if (typeof navigator === "undefined" || !navigator.mediaCapabilities) return false;

  try {
    const result = await navigator.mediaCapabilities.decodingInfo(HLG_PROGRESSIVE_CONFIGURATION);
    return result.supported && result.smooth;
  } catch {
    return false;
  }
}

/** Probes the exact clean HLG base-layer shape consumed by hls.js. */
export async function probeHLSHLGPlaybackSupport(): Promise<boolean> {
  if (
    typeof navigator === "undefined" ||
    !navigator.mediaCapabilities ||
    typeof MediaSource === "undefined"
  ) {
    return false;
  }
  try {
    if (!MediaSource.isTypeSupported(HLG_HLS_CONFIGURATION.video.contentType)) return false;
    const result = await navigator.mediaCapabilities.decodingInfo(HLG_HLS_CONFIGURATION);
    return result.supported && result.smooth;
  } catch {
    return false;
  }
}

function testMediaType(mime: string): boolean {
  if (typeof MediaSource !== "undefined") {
    try {
      if (MediaSource.isTypeSupported(mime)) return true;
    } catch {
      // Fall through to the media element probe.
    }
  }

  if (typeof document === "undefined") return false;
  try {
    return (
      document.createElement(mime.startsWith("audio/") ? "audio" : "video").canPlayType(mime) !== ""
    );
  } catch {
    return false;
  }
}

function testMediaSourceType(mime: string): boolean {
  if (typeof MediaSource === "undefined") return false;
  try {
    return MediaSource.isTypeSupported(mime);
  } catch {
    return false;
  }
}

function testMediaElementType(mime: string): boolean {
  if (typeof document === "undefined") return false;
  try {
    // `maybe` only recognizes the container/type. Structured Dolby Vision
    // claims require the media element's definitive answer for the exact
    // sample entry.
    return document.createElement("video").canPlayType(mime) === "probably";
  } catch {
    return false;
  }
}

/**
 * Probes what this browser will admit to decoding.
 *
 * Every answer here comes from `MediaSource.isTypeSupported(...)` or
 * `HTMLMediaElement.canPlayType(...)`, which is why the v3 capability block
 * built from this probe is `declared` evidence and never claims hardware decode
 * detail it cannot observe. The screen-derived resolution and the HDR media
 * queries are hints about the *output*, not the decoder, and the server treats
 * them as such.
 */
export function probeWebCapabilities(): WebCapabilityProbe {
  const codecsVideo: string[] = [];
  const hlsCodecsVideo: string[] = [];
  const codecsAudio: string[] = [];
  const containers: string[] = [];

  // Test containers.
  for (const [name, mimeTypes] of Object.entries(CONTAINER_MAP)) {
    if (mimeTypes.some(testMediaType)) {
      containers.push(name);
    }
  }

  // Test video codecs (in mp4 container).
  for (const [name, codec] of Object.entries(VIDEO_CODEC_MAP)) {
    const mime = `video/mp4; codecs="${codec}"`;
    if (testMediaType(mime)) {
      codecsVideo.push(name);
    }
    if (testMediaSourceType(mime)) hlsCodecsVideo.push(name);
  }

  // Test audio codecs.
  for (const [name, mimeTypes] of Object.entries(AUDIO_CODEC_MAP)) {
    if (mimeTypes.some(testMediaType)) {
      codecsAudio.push(name);
    }
  }

  // `screen` reports logical CSS pixels; a 2160p-class panel on a 2x display
  // measures 1080p without the device pixel ratio applied.
  const pixelRatio =
    typeof window !== "undefined" &&
    typeof window.devicePixelRatio === "number" &&
    Number.isFinite(window.devicePixelRatio) &&
    window.devicePixelRatio > 0
      ? window.devicePixelRatio
      : 1;
  const maxResolution =
    typeof screen !== "undefined"
      ? detectMaxResolutionFromScreen(screen.width * pixelRatio, screen.height * pixelRatio)
      : "1080p";

  // HDR detection (best effort). Wrap matchMedia so it keeps its Window
  // receiver — invoking a detached reference throws in some browsers.
  const hdr = detectHDRFromMatchMedia(
    typeof matchMedia !== "undefined" ? (query) => matchMedia(query) : undefined,
  );
  // Decoder capability and active-output HDR are separate facts: browsers
  // tone-map HDR content onto SDR outputs, and Safari 26 reports
  // `dynamic-range: standard` even on an XDR panel. Exact positive decode
  // evidence must not be discarded because the coarse output query says no, so
  // the sample-entry probes run unconditionally and `hdr` stays a best-effort
  // output signal only.
  const dolbyVisionProfileLevels: Array<{
    profile: number;
    max_level: number;
    bl_compatibility_ids?: number[];
  }> = [];
  for (const [profileValue, probe] of Object.entries(DOLBY_VISION_PROFILE_PROBES)) {
    const profile = Number(profileValue);
    const maxLevel = probe.levels.find((level) =>
      testMediaElementType(
        `video/mp4; codecs="dvh1.${String(profile).padStart(2, "0")}.${String(level).padStart(2, "0")}"`,
      ),
    );
    if (maxLevel === undefined) continue;
    dolbyVisionProfileLevels.push({
      profile,
      max_level: maxLevel,
      ...(probe.blCompatibilityIds ? { bl_compatibility_ids: probe.blCompatibilityIds } : {}),
    });
  }
  const dolbyVisionProfiles = dolbyVisionProfileLevels.map(({ profile }) => profile);
  const progressiveCodecsVideo = [...codecsVideo];
  if (dolbyVisionProfiles.length > 0 && !progressiveCodecsVideo.includes("hevc")) {
    // Every Dolby Vision profile probed above uses an HEVC base layer. The
    // planner requires the flat base-codec claim as well as the HDR profile,
    // but this media-element evidence must not leak into hls.js' MSE path.
    progressiveCodecsVideo.push("hevc");
  }
  const hdrDetails = {
    // Generic HDR output eligibility does not prove either static HDR format.
    // Only publish the exact Dolby Vision formats tested above.
    hdr10: false,
    hdr10_plus: false,
    hlg: false,
    dolby_vision_profiles: dolbyVisionProfiles,
    dolby_vision_profile_levels: dolbyVisionProfileLevels,
  };
  const hlsHDRDetails = {
    hdr10: false,
    hdr10_plus: false,
    hlg: false,
    dolby_vision_profiles: [] as number[],
    dolby_vision_profile_levels: [],
  };

  return {
    containers,
    codecsVideo,
    progressiveCodecsVideo,
    hlsCodecsVideo,
    codecsAudio,
    maxResolution,
    hdr,
    hdrDetails,
    hlsHDRDetails,
    hls: detectHLSSupport(),
    nativeHls: detectNativeHLSSupport(),
  };
}

export interface WebCapabilityDetection {
  probe: WebCapabilityProbe;
  settled: boolean;
}

interface ExactWebCapabilityProbe {
  hdr10: boolean;
  hlg: boolean;
  hlsHDR10: boolean;
  hlsHLG: boolean;
}

const CAPABILITY_PROBE_TIMEOUT_MS = 250;
const CAPABILITY_CACHE_KEY = "silo.playback.exact-web-capabilities.v1";
const capabilityCacheEnabled = import.meta.env.MODE !== "test";
let exactCapabilityCache: ExactWebCapabilityProbe | null = null;
let exactCapabilityPromise: Promise<ExactWebCapabilityProbe> | null = null;
let exactCapabilityGeneration = 0;

function boundedCapabilityProbe(probe: Promise<boolean>): Promise<boolean> {
  return new Promise((resolve) => {
    let complete = false;
    const timeout = window.setTimeout(() => {
      if (complete) return;
      complete = true;
      resolve(false);
    }, CAPABILITY_PROBE_TIMEOUT_MS);
    void probe.then(
      (supported) => {
        if (complete) return;
        complete = true;
        window.clearTimeout(timeout);
        resolve(supported);
      },
      () => {
        if (complete) return;
        complete = true;
        window.clearTimeout(timeout);
        resolve(false);
      },
    );
  });
}

function readExactCapabilityCache(): ExactWebCapabilityProbe | null {
  if (exactCapabilityCache) return exactCapabilityCache;
  if (!capabilityCacheEnabled || typeof sessionStorage === "undefined") return null;
  try {
    const value = JSON.parse(
      sessionStorage.getItem(CAPABILITY_CACHE_KEY) ?? "null",
    ) as Partial<ExactWebCapabilityProbe> | null;
    if (
      value &&
      typeof value.hdr10 === "boolean" &&
      typeof value.hlg === "boolean" &&
      typeof value.hlsHDR10 === "boolean" &&
      typeof value.hlsHLG === "boolean"
    ) {
      exactCapabilityCache = value as ExactWebCapabilityProbe;
      return exactCapabilityCache;
    }
  } catch {
    // A disabled or corrupted session cache is only a performance miss.
  }
  return null;
}

function probeExactWebCapabilities(): Promise<ExactWebCapabilityProbe> {
  const cached = readExactCapabilityCache();
  if (cached) return Promise.resolve(cached);
  if (capabilityCacheEnabled && exactCapabilityPromise) return exactCapabilityPromise;

  const generation = exactCapabilityGeneration;
  const pending = Promise.all([
    boundedCapabilityProbe(probeHDR10PlaybackSupport()),
    boundedCapabilityProbe(probeHLGPlaybackSupport()),
    boundedCapabilityProbe(probeHLSHDR10PlaybackSupport()),
    boundedCapabilityProbe(probeHLSHLGPlaybackSupport()),
  ]).then(([hdr10, hlg, hlsHDR10, hlsHLG]) => {
    const result = { hdr10, hlg, hlsHDR10, hlsHLG };
    if (capabilityCacheEnabled && generation === exactCapabilityGeneration) {
      exactCapabilityCache = result;
      try {
        sessionStorage.setItem(CAPABILITY_CACHE_KEY, JSON.stringify(result));
      } catch {
        // Playback still uses the in-memory result when storage is disabled.
      }
    }
    return result;
  });
  if (capabilityCacheEnabled) exactCapabilityPromise = pending;
  return pending;
}

function invalidateExactCapabilityCache(): void {
  exactCapabilityGeneration += 1;
  exactCapabilityCache = null;
  exactCapabilityPromise = null;
  if (!capabilityCacheEnabled || typeof sessionStorage === "undefined") return;
  try {
    sessionStorage.removeItem(CAPABILITY_CACHE_KEY);
  } catch {
    // A disabled session cache does not affect the live probe.
  }
}

function applyExactWebCapabilities(
  next: WebCapabilityProbe,
  { hdr10, hlg, hlsHDR10, hlsHLG }: ExactWebCapabilityProbe,
): WebCapabilityProbe {
  const progressiveCodecsVideo =
    (hdr10 || hlg) && !next.progressiveCodecsVideo.includes("hevc")
      ? [...next.progressiveCodecsVideo, "hevc"]
      : next.progressiveCodecsVideo;
  const hlsCodecsVideo =
    (hlsHDR10 || hlsHLG) && !next.hlsCodecsVideo.includes("hevc")
      ? [...next.hlsCodecsVideo, "hevc"]
      : next.hlsCodecsVideo;
  return {
    ...next,
    progressiveCodecsVideo,
    hlsCodecsVideo,
    hdrDetails: {
      ...next.hdrDetails,
      ...(hdr10
        ? {
            hdr10: true,
            hdr10_max_width: 3840,
            hdr10_max_height: 2160,
            hdr10_max_frame_rate: 24,
            hdr10_max_bitrate_kbps: 80_000,
          }
        : {}),
      ...(hlg ? { hlg: true } : {}),
    },
    hlsHDRDetails: {
      ...next.hlsHDRDetails,
      ...(hlsHDR10
        ? {
            hdr10: true,
            hdr10_max_width: 3840,
            hdr10_max_height: 2160,
            hdr10_max_frame_rate: 24,
            hdr10_max_bitrate_kbps: 80_000,
          }
        : {}),
      ...(hlsHLG ? { hlg: true } : {}),
    },
  };
}

// The web-player module loads before a viewer opens playback. Start the
// bounded decoder probes immediately so an initial play or resume normally
// consumes a ready result instead of paying probe latency at click time.
if (capabilityCacheEnabled && typeof window !== "undefined") {
  void probeExactWebCapabilities();
}

/**
 * Keeps the browser capability probe current with the active output route.
 * Moving a window between SDR and HDR displays can change the media-query
 * result without remounting the player, so refresh when either query changes.
 */
export function useCodecDetectionState(): WebCapabilityDetection {
  const [detection, setDetection] = useState<WebCapabilityDetection>(() => {
    const probe = probeWebCapabilities();
    const cached = readExactCapabilityCache();
    return cached
      ? { probe: applyExactWebCapabilities(probe, cached), settled: true }
      : { probe, settled: false };
  });

  useEffect(() => {
    let disposed = false;
    let probeGeneration = 0;
    const queries =
      typeof matchMedia === "undefined"
        ? []
        : [matchMedia("(dynamic-range: high)"), matchMedia("(video-dynamic-range: high)")];
    const refresh = () => {
      const generation = ++probeGeneration;
      const next = probeWebCapabilities();

      const cached = readExactCapabilityCache();
      if (cached) {
        setDetection({ probe: applyExactWebCapabilities(next, cached), settled: true });
        return;
      }

      // Keep the last verified capability bytes while exposing that a newer
      // output probe is pending. Publishing `next` here would reintroduce the
      // transient no-HDR route that caused fallback and resume races.
      setDetection((current) => (current.settled ? { ...current, settled: false } : current));

      // Publish only the settled snapshot. An intermediate "no HDR" state
      // used to open a 1080p fallback, then replace it milliseconds later when
      // the async probe completed. If that replacement failed, the original
      // resume anchor was lost. A bounded probe keeps startup finite without
      // leaking transient capability state into an active session.
      void probeExactWebCapabilities().then((exact) => {
        if (disposed || generation !== probeGeneration) return;
        setDetection({
          settled: true,
          probe: applyExactWebCapabilities(next, exact),
        });
      });
    };
    refresh();
    const refreshForOutputChange = () => {
      // Media Capabilities can change when macOS moves a window to another
      // display. Keep navigation/resume fast with the cache, but never reuse
      // it across an actual output transition.
      invalidateExactCapabilityCache();
      refresh();
    };
    for (const query of queries) {
      if (typeof query.addEventListener === "function")
        query.addEventListener("change", refreshForOutputChange);
      else query.addListener?.(refreshForOutputChange);
    }
    return () => {
      disposed = true;
      for (const query of queries) {
        if (typeof query.removeEventListener === "function")
          query.removeEventListener("change", refreshForOutputChange);
        else query.removeListener?.(refreshForOutputChange);
      }
    };
  }, []);

  return detection;
}

export function useCodecDetection(): WebCapabilityProbe {
  return useCodecDetectionState().probe;
}
