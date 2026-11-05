import { useEffect, useState } from "react";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { api, queryString } from "./api";
import type { Page } from "./types";

export function useList<T>(
  resource: string,
  extra: Record<string, string> = {},
) {
  const [query, setQuery] = useState("");
  const [debounced, setDebounced] = useState("");
  const [status, updateStatus] = useState("all");
  const [sort, updateSort] = useState("createdAt");
  const [order, updateOrder] = useState("desc");
  const [page, setPage] = useState(1);
  const [pageSize, updatePageSize] = useState(10);
  useEffect(() => {
    const timer = setTimeout(() => {
      setDebounced(query);
      setPage(1);
    }, 250);
    return () => clearTimeout(timer);
  }, [query]);
  const params = {
    page,
    pageSize,
    q: debounced,
    status,
    sort,
    order,
    ...extra,
  };
  const result = useQuery({
    queryKey: ["admin", resource, params],
    queryFn: ({ signal }) =>
      api<Page<T>>(`/${resource}${queryString(params)}`, { signal }),
    placeholderData: keepPreviousData,
  });
  useEffect(() => {
    if (
      result.data &&
      !result.isPlaceholderData &&
      page > 1 &&
      result.data.total <= (page - 1) * pageSize
    )
      setPage(Math.max(1, Math.ceil(result.data.total / pageSize)));
  }, [page, pageSize, result.data, result.isPlaceholderData]);
  return {
    ...result,
    query,
    setQuery,
    status,
    setStatus: (value: string) => {
      updateStatus(value);
      setPage(1);
    },
    sort,
    setSort: (value: string) => {
      updateSort(value);
      setPage(1);
    },
    order,
    setOrder: (value: string) => {
      updateOrder(value);
      setPage(1);
    },
    page,
    setPage,
    pageSize,
    setPageSize: (value: number) => {
      updatePageSize(value);
      setPage(1);
    },
    reset: () => {
      setQuery("");
      setDebounced("");
      updateStatus("all");
      setPage(1);
    },
  };
}
