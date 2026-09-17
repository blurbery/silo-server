import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";

import { adminRefreshPerson, adminUpdatePerson } from "@/api/v2/people";
import type { Person, UpdatePersonRequest } from "@/api/types";
import { refreshPerson, searchPeople, type PersonRefreshResult } from "@/api/v2/people";

import { personKeys } from "./keys";

export function usePersonSearch(query: string, limit = 20, enabled = true) {
  const normalizedQuery = query.trim();

  return useQuery({
    queryKey: personKeys.search(normalizedQuery, limit),
    queryFn: ({ signal }) => searchPeople(normalizedQuery, limit, { signal }),
    enabled: enabled && normalizedQuery.length > 0,
    staleTime: 5 * 60 * 1000,
  });
}

type RefreshPersonResult =
  | {
      mode: "admin";
      person: Person;
    }
  | {
      mode: "queued";
      response: PersonRefreshResult;
    };

export function useRefreshPerson(id: string | undefined, isAdmin: boolean) {
  const queryClient = useQueryClient();

  return useMutation({
    retry: false,
    mutationFn: async (): Promise<RefreshPersonResult> => {
      if (!id) {
        throw new Error("Person ID is required");
      }

      if (isAdmin) {
        return {
          mode: "admin",
          person: await adminRefreshPerson(id),
        };
      }

      return {
        mode: "queued",
        response: await refreshPerson(id),
      };
    },
    onSuccess: async (result) => {
      if (result.mode === "admin" && id) {
        queryClient.setQueryData(personKeys.detail(id), result.person);
        await queryClient.invalidateQueries({ queryKey: personKeys.detail(id) });
        toast.success("Person metadata refreshed");
        return;
      }

      toast.success("Person refresh queued");
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Refresh failed");
    },
  });
}

export function useUpdatePersonMetadata(id: string | undefined) {
  const queryClient = useQueryClient();

  return useMutation({
    retry: false,
    mutationFn: (data: UpdatePersonRequest) => {
      if (!id) {
        throw new Error("Person ID is required");
      }

      return adminUpdatePerson(id, data);
    },
    onSuccess: async (updatedPerson) => {
      if (!id) {
        return;
      }

      queryClient.setQueryData(personKeys.detail(id), updatedPerson);
      await queryClient.invalidateQueries({ queryKey: personKeys.detail(id) });
      toast.success("Person metadata saved");
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to save metadata");
    },
  });
}
