import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/api/client";
import type { CommunityRatingReaction, CommunityRatingsResponse, ItemDetail } from "@/api/types";
import { invalidateRatingSurfaceQueries } from "./ratingsSurfaceRefresh";
import { ratingKeys } from "./keys";
import {
  cancelItemDetailQueries,
  isItemDetailQueryKey,
  updateCatalogItemDetail,
} from "./mediaSurfaceRefresh";

export function useSetRating(itemId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (rating: number) =>
      api(`/ratings/${itemId}`, {
        method: "PUT",
        body: JSON.stringify({ rating }),
      }),
    onMutate: async (rating: number) => {
      await cancelItemDetailQueries(queryClient, itemId);
      const previous = queryClient.getQueriesData<ItemDetail>({
        predicate: (query) => isItemDetailQueryKey(query.queryKey, itemId),
      });
      updateCatalogItemDetail(queryClient, itemId, (detail) => ({
        ...detail,
        user_rating: rating,
      }));
      return { previous };
    },
    onError: (_err, _vars, context) => {
      for (const [queryKey, value] of context?.previous ?? []) {
        queryClient.setQueryData(queryKey, value);
      }
    },
    onSettled: () => {
      return invalidateRatingSurfaceQueries(queryClient, itemId);
    },
  });
}

export function useDeleteRating(itemId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => api(`/ratings/${itemId}`, { method: "DELETE" }),
    onMutate: async () => {
      await cancelItemDetailQueries(queryClient, itemId);
      const previous = queryClient.getQueriesData<ItemDetail>({
        predicate: (query) => isItemDetailQueryKey(query.queryKey, itemId),
      });
      updateCatalogItemDetail(queryClient, itemId, (detail) => ({
        ...detail,
        user_rating: null,
      }));
      return { previous };
    },
    onError: (_err, _vars, context) => {
      for (const [queryKey, value] of context?.previous ?? []) {
        queryClient.setQueryData(queryKey, value);
      }
    },
    onSettled: () => {
      return invalidateRatingSurfaceQueries(queryClient, itemId);
    },
  });
}

export function useCommunityRatings(itemId: string) {
  return useQuery({
    queryKey: ratingKeys.community(itemId),
    queryFn: () => api<CommunityRatingsResponse>(`/ratings/${itemId}/community?limit=100`),
    enabled: itemId.length > 0,
    staleTime: 30_000,
  });
}

interface CommunityReactionMutation {
  ratingKey: string;
  reaction: CommunityRatingReaction | null;
}

export function useSetCommunityRatingReaction(itemId: string) {
  const queryClient = useQueryClient();
  const queryKey = ratingKeys.community(itemId);

  return useMutation({
    mutationFn: ({ ratingKey, reaction }: CommunityReactionMutation) => {
      const path = `/ratings/${itemId}/community/${encodeURIComponent(ratingKey)}/reaction`;
      if (reaction === null) {
        return api<void>(path, { method: "DELETE" });
      }
      return api<void>(path, {
        method: "PUT",
        body: JSON.stringify({ reaction }),
      });
    },
    onMutate: async ({ ratingKey, reaction }: CommunityReactionMutation) => {
      await queryClient.cancelQueries({ queryKey });
      const previous = queryClient.getQueryData<CommunityRatingsResponse>(queryKey);
      queryClient.setQueryData<CommunityRatingsResponse>(queryKey, (current) => {
        if (!current) return current;
        return {
          ...current,
          ratings: current.ratings.map((entry) => {
            if (entry.key !== ratingKey) return entry;
            return {
              ...entry,
              up_count: Math.max(
                0,
                entry.up_count -
                  (entry.viewer_reaction === "up" ? 1 : 0) +
                  (reaction === "up" ? 1 : 0),
              ),
              down_count: Math.max(
                0,
                entry.down_count -
                  (entry.viewer_reaction === "down" ? 1 : 0) +
                  (reaction === "down" ? 1 : 0),
              ),
              viewer_reaction: reaction ?? undefined,
            };
          }),
        };
      });
      return { previous };
    },
    onError: (_error, _variables, context) => {
      if (context?.previous) {
        queryClient.setQueryData(queryKey, context.previous);
      }
    },
    onSettled: () => queryClient.invalidateQueries({ queryKey }),
  });
}
