import { useMemo, useCallback } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/api/client";
import {
  effectiveSettingsQueryKey,
  isDefinitiveSettingMutationRejection,
  useEffectiveSettings,
  useSetSettingValue,
  type EffectiveSettingsMap,
} from "@/hooks/queries/settingValues";
import type { SettingIdentity } from "@/hooks/queries/settingValues";
import { SETTING_KEYS, type SettingKey } from "@/lib/settingsContract";
import { settingsKeys } from "@/hooks/queries/keys";
import { storage } from "@/utils/storage";
import { parseOverlayPrefs, serializeOverlayPrefs, type CardOverlayPrefs } from "@/lib/overlays";
import {
  normalizeCardQuickActionMode,
  type EnabledCardQuickActionMode,
} from "@/lib/cardQuickActions";

/** Card overlay preferences are profile-wide in the contract (no device scope). */
const PROFILE_SCOPE: SettingIdentity = { scope: "profile" };

const OVERLAY_KEYS = [
  SETTING_KEYS.UI_CARD_OVERLAYS,
  SETTING_KEYS.UI_CARD_QUICK_ACTIONS,
  SETTING_KEYS.UI_CARD_QUICK_ACTIONS_ENABLED,
] as const;

interface OverlayConfig {
  enabled: boolean;
  defaults?: string;
  quick_actions_enabled?: boolean;
  quick_actions_default?: string;
}

// The overlay-badge kill switch and server-wide defaults live in
// server_settings, not the user-settings contract, so this endpoint stays
// alongside the canonical values API.
function useOverlayConfig() {
  return useQuery({
    queryKey: settingsKeys.overlayConfig(),
    queryFn: () => api<OverlayConfig>("/settings/overlay-config"),
    staleTime: 60_000,
  });
}

export function useOverlayPrefs() {
  // The effective endpoint requires a profile header; without one the user
  // has no stored preference and the admin defaults apply on their own.
  const profileId = storage.get(storage.KEYS.PROFILE_ID);
  const hasProfile = Boolean(profileId);
  const { data: effective, isLoading: userLoading } = useEffectiveSettings({
    keys: OVERLAY_KEYS,
    enabled: hasProfile,
  });
  const { data: config, isLoading: configLoading } = useOverlayConfig();
  const setValue = useSetSettingValue();
  const queryClient = useQueryClient();
  const effectiveQueryKey = useMemo(
    () => effectiveSettingsQueryKey({ keys: OVERLAY_KEYS, profileId: profileId ?? undefined }),
    // The active profile is part of effectiveSettingsQueryKey. Recompute the
    // target when profile selection changes rather than writing into the
    // previous profile's cache entry.
    [profileId],
  );

  const setProfileValue = useCallback(
    (key: SettingKey, value: unknown) => {
      const previous = queryClient.getQueryData<EffectiveSettingsMap>(effectiveQueryKey);
      queryClient.setQueryData<EffectiveSettingsMap>(effectiveQueryKey, (current) => ({
        ...current,
        [key]: { key, value, source: "profile", scope: "profile" },
      }));
      setValue.mutate(
        { key, value, identity: PROFILE_SCOPE },
        {
          // The shared mutation invalidates on success and ambiguous errors.
          // A definitive rejection never reached storage, so restore the exact
          // key that this optimistic write replaced without clobbering another
          // overlay control that may have saved concurrently.
          onError: (error) => {
            if (isDefinitiveSettingMutationRejection(error)) {
              queryClient.setQueryData<EffectiveSettingsMap>(effectiveQueryKey, (current) => {
                if (current?.[key]?.value !== value) return current;
                const restored = { ...current };
                const previousSetting = previous?.[key];
                if (previousSetting) restored[key] = previousSetting;
                else delete restored[key];
                return restored;
              });
            }
          },
        },
      );
    },
    [effectiveQueryKey, queryClient, setValue],
  );

  // The contract default is null — "no preference expressed" — which is what
  // lets the server-wide admin default apply; a stored value wins outright.
  const userValue = effective?.[SETTING_KEYS.UI_CARD_OVERLAYS]?.value ?? null;
  const quickActionUserValue = effective?.[SETTING_KEYS.UI_CARD_QUICK_ACTIONS]?.value ?? null;
  const quickActionsEnabledUserValue =
    effective?.[SETTING_KEYS.UI_CARD_QUICK_ACTIONS_ENABLED]?.value;

  const prefs = useMemo(() => {
    // User setting takes priority; fall back to admin defaults
    const source = userValue ?? config?.defaults ?? null;
    return parseOverlayPrefs(source);
  }, [userValue, config?.defaults]);

  // Admin kill switch: if disabled server-wide, return null prefs
  const enabled = config?.enabled !== false;
  // Match the overlay-badge rules: the server setting is a policy gate, while
  // a profile may only customize quick actions when that gate is open. Keep
  // the stored profile preference intact so it becomes effective again if an
  // administrator later re-enables quick actions.
  const quickActionsGloballyEnabled = config?.quick_actions_enabled !== false;
  const quickActionsEnabledByProfile =
    typeof quickActionsEnabledUserValue === "boolean" ? quickActionsEnabledUserValue : true;
  const quickActionsEnabled = quickActionsGloballyEnabled && quickActionsEnabledByProfile;
  const configuredQuickActionMode = normalizeCardQuickActionMode(
    quickActionUserValue,
    normalizeCardQuickActionMode(config?.quick_actions_default),
  );

  const setPrefs = useCallback(
    (next: CardOverlayPrefs) => {
      // Avoid a network round-trip and downstream re-render cascade when
      // the user toggles a control to its current value. Comparison goes
      // through the parser so key ordering in the stored JSON is irrelevant.
      if (
        userValue != null &&
        serializeOverlayPrefs(parseOverlayPrefs(userValue)) === serializeOverlayPrefs(next)
      ) {
        return;
      }
      setProfileValue(SETTING_KEYS.UI_CARD_OVERLAYS, next);
    },
    [userValue, setProfileValue],
  );

  const setQuickActionMode = useCallback(
    (next: EnabledCardQuickActionMode) => {
      if (!quickActionsGloballyEnabled) return;
      if (
        quickActionUserValue != null &&
        normalizeCardQuickActionMode(quickActionUserValue) === next
      ) {
        return;
      }
      setProfileValue(SETTING_KEYS.UI_CARD_QUICK_ACTIONS, next);
    },
    [quickActionUserValue, quickActionsGloballyEnabled, setProfileValue],
  );

  const setQuickActionsEnabled = useCallback(
    (next: boolean) => {
      if (!quickActionsGloballyEnabled) return;
      if (
        typeof quickActionsEnabledUserValue === "boolean" &&
        quickActionsEnabledUserValue === next
      ) {
        return;
      }
      setProfileValue(SETTING_KEYS.UI_CARD_QUICK_ACTIONS_ENABLED, next);
    },
    [quickActionsEnabledUserValue, quickActionsGloballyEnabled, setProfileValue],
  );

  // While either query is in flight, report null prefs instead of built-in
  // defaults: rendering defaults first would flash badges that vanish (or
  // change) the moment the user's own config or the admin kill switch loads.
  const isLoading = (hasProfile && userLoading) || configLoading;

  return {
    prefs: enabled && !isLoading ? prefs : null,
    setPrefs,
    quickActionMode:
      quickActionsEnabled && !isLoading ? configuredQuickActionMode : ("none" as const),
    quickActionPreference: configuredQuickActionMode,
    setQuickActionMode,
    quickActionsEnabled,
    quickActionsGloballyEnabled,
    setQuickActionsEnabled,
    isLoading,
    enabled,
  };
}
