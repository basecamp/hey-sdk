import type { operations } from "./generated/schema.js";
export type OperationName = keyof operations;
type Parameters<T> = T extends { parameters: infer P }
  ? {
      [K in keyof P as K extends "path" | "query"
        ? [NonNullable<P[K]>] extends [never]
          ? never
          : K
        : never]: P[K];
    }
  : {};
type Body<T> = T extends {
  requestBody: { content: { "application/json": infer B } };
}
  ? { body: B }
  : {};
export type OperationInput<K extends OperationName> = Parameters<
  operations[K]
> &
  Body<operations[K]>;
type Content<T> = T extends { content: { "application/json": infer C } }
  ? C
  : undefined;
type Success<T> = {
  [K in keyof T]: K extends string | number
    ? `${K}` extends `2${string}`
      ? Content<T[K]>
      : never
    : never;
}[keyof T];
export type OperationData<K extends OperationName> = Success<
  operations[K]["responses"]
>;
export type OperationResponse<K extends OperationName> = ApiResponse<
  OperationData<K>
>;
export interface RequestOptions {
  signal?: AbortSignal;
  /** Request Rails' explicit .json representation using the generated route. */
  format?: "json";
}
export interface ApiResponse<T> {
  /** Empty-on status and empty successful responses have no data. */
  data: T | undefined;
  status: number;
  headers: Headers;
  totalCount?: number | bigint;
  nextPage?: string;
  /** Validated same-origin URL; sync bookmarks in data are never followed. */
  nextUrl?: string;
}
export interface Transport {
  execute<K extends OperationName>(
    operation: K,
    input: OperationInput<K>,
    options?: RequestOptions,
  ): Promise<OperationResponse<K>>;
}
