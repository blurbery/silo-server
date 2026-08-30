import { useCallback, useState } from "react";

interface ItemDetailsGateState {
  locationKey: string;
  pathname: string;
  gatesItemDetails: boolean;
  pendingLocationKey: string | null;
  enteredItemFromHome: boolean;
}

/**
 * Keeps item details lightweight while a desktop route entry collapses the
 * sidebar. The gate follows committed route transitions instead of individual
 * click handlers, so POP and programmatic navigation receive the same
 * treatment and abandoned navigation cannot leave a stale global lock.
 */
export function useSidebarItemDetailsGate(
  locationKey: string,
  pathname: string,
  gatesItemDetails: boolean,
) {
  const [state, setState] = useState<ItemDetailsGateState>({
    locationKey,
    pathname,
    gatesItemDetails,
    pendingLocationKey: null,
    enteredItemFromHome: false,
  });

  let currentState = state;
  if (
    state.locationKey !== locationKey ||
    state.pathname !== pathname ||
    state.gatesItemDetails !== gatesItemDetails
  ) {
    const enteringItem = gatesItemDetails && !state.gatesItemDetails;
    currentState = {
      locationKey,
      pathname,
      gatesItemDetails,
      pendingLocationKey: enteringItem ? locationKey : null,
      enteredItemFromHome: enteringItem && state.pathname === "/",
    };
    // React discards this render and retries before rendering children, so the
    // item route never briefly receives `itemDetailsReady=true` on entry.
    setState(currentState);
  }

  const reveal = useCallback((expectedLocationKey: string) => {
    setState((current) =>
      current.pendingLocationKey === expectedLocationKey
        ? { ...current, pendingLocationKey: null }
        : current,
    );
  }, []);

  return {
    itemDetailsReady: !gatesItemDetails || currentState.pendingLocationKey === null,
    pendingLocationKey: currentState.pendingLocationKey,
    enteredItemFromHome: currentState.enteredItemFromHome,
    reveal,
  };
}
