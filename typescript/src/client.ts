import { setTimeout as delay } from "node:timers/promises";
import { createHash } from "node:crypto";
import {
  GeneratedOperations,
  operationMetadata,
} from "./generated/operations.js";
import type { components } from "./generated/schema.js";
import type {
  ApiResponse,
  OperationName,
  OperationInput,
  OperationResponse,
  RequestOptions,
  Transport,
} from "./protocol.js";
import { HeyError, responseError } from "./errors.js";
import {
  assertSafeNumbers,
  networkFailure,
  nextLink,
  parseJSON,
  readBounded,
  sameOriginURL,
  secureURL,
  stringifyJSON,
} from "./security.js";
import { USER_AGENT } from "./version.js";

export interface TokenProvider {
  getToken(): string | Promise<string>;
  /** Renew after a definite 401 rejection. Called at most once per request. */
  refresh?(): void | Promise<void>;
}
export interface ClientOptions {
  token: string | TokenProvider;
  baseUrl?: string;
  userAgent?: string;
  fetch?: typeof globalThis.fetch;
  maxPages?: number;
  maxResponseBodyBytes?: number;
  timeoutMs?: number;
  /** Bound the model's retry attempts further; zero disables transient retries. */
  maxRetries?: number;
  /** Opt-in bounded in-memory ETag revalidation, partitioned by token and URL. */
  cache?: boolean;
}
type Identity = components["schemas"]["Identity"];
type Metadata = {
  method: string;
  path: string;
  parameters: readonly {
    name: string;
    in: string;
    required: boolean;
    type?: string;
  }[];
  bodyRequired: boolean;
  safe: boolean;
  actingSender: boolean;
  actingUser: boolean;
  retry?: {
    maxAttempts: number;
    baseDelayMs: number;
    backoff: string;
    retryOn: readonly number[];
  };
  pagination?: { style: string };
  emptyOn?: readonly number[];
};
type RawInput = {
  path?: Record<string, unknown>;
  query?: Record<string, unknown>;
  body?: unknown;
};
/** Cancel only this wait, not the provider work shared with other requests. */
function waitForCredentials<T>(
  pending: Promise<T>,
  signal: AbortSignal,
): Promise<T> {
  return new Promise((resolve, reject) => {
    const cleanup = () => signal.removeEventListener("abort", aborted);
    const aborted = () => {
      cleanup();
      reject(signal.reason);
    };
    signal.addEventListener("abort", aborted, { once: true });
    if (signal.aborted) aborted();
    pending.then(
      (value) => {
        cleanup();
        resolve(value);
      },
      (error: unknown) => {
        cleanup();
        reject(error);
      },
    );
  });
}
class Credentials {
  private refreshing?: Promise<void>;
  private generation = 0;
  constructor(readonly provider: TokenProvider) {}
  async token() {
    // An asynchronous read may return an old token after another request renews.
    const generation = this.generation;
    return {
      value: await this.provider.getToken(),
      generation,
    };
  }
  async refresh(generation: number) {
    if (!this.provider.refresh || generation !== this.generation) return;
    this.refreshing ??= Promise.resolve()
      .then(() => this.provider.refresh!())
      .then(() => {
        this.generation++;
      })
      .finally(() => {
        this.refreshing = undefined;
      });
    await this.refreshing;
  }
}
class HttpTransport implements Transport {
  readonly base: URL;
  readonly fetch: typeof globalThis.fetch;
  readonly maxPages: number;
  readonly maxBytes: number;
  readonly timeout: number;
  accountId?: number | bigint;
  identity?: Identity;
  private readonly cache = new Map<
    string,
    { text: string; headers: Headers }
  >();
  constructor(
    readonly options: ClientOptions,
    readonly credentials: Credentials,
  ) {
    this.base = secureURL(options.baseUrl ?? "https://app.hey.com");
    if (this.base.search || !["", "/"].includes(this.base.pathname))
      throw new HeyError(
        "usage",
        "baseUrl must be an origin without a path or query",
      );
    this.fetch = options.fetch ?? globalThis.fetch;
    this.maxPages = options.maxPages ?? 100;
    this.maxBytes = options.maxResponseBodyBytes ?? 16 * 1024 * 1024;
    this.timeout = options.timeoutMs ?? 30_000;
    for (const value of [this.maxPages, this.maxBytes, this.timeout])
      if (!Number.isSafeInteger(value) || value <= 0)
        throw new HeyError("usage", "Limits must be positive safe integers");
    if (this.timeout > 2_147_483_647)
      throw new HeyError(
        "usage",
        "timeoutMs exceeds the reliable Node.js timer maximum",
      );
    if (
      options.maxRetries !== undefined &&
      (!Number.isSafeInteger(options.maxRetries) || options.maxRetries < 0)
    )
      throw new HeyError("usage", "maxRetries must be a nonnegative integer");
  }
  url(operation: OperationName, input: RawInput, options: RequestOptions): URL {
    const meta: Metadata = operationMetadata[operation];
    assertSafeNumbers(input);
    let path = meta.path;
    for (const p of meta.parameters) {
      const value = (p.in === "path" ? input.path : input.query)?.[p.name];
      if (p.required && (value === undefined || value === null || value === ""))
        throw new HeyError("usage", `Missing ${p.in} parameter ${p.name}`);
      if (p.in !== "path") continue;
      if (
        p.type === "integer" &&
        !(
          (typeof value === "number" && Number.isSafeInteger(value)) ||
          typeof value === "bigint"
        )
      )
        throw new HeyError("usage", `${p.name} must be an integer`);
      const label = String(value);
      if (
        !label ||
        label === "." ||
        label === ".." ||
        /[/\\%?#\x00-\x1f]/.test(label)
      )
        throw new HeyError("usage", `Unsafe path parameter ${p.name}`);
      path = path.replace(`{${p.name}}`, encodeURIComponent(label));
    }
    if (options.format === "json" && !path.endsWith(".json")) path += ".json";
    const url = new URL(path, this.base);
    for (const [name, value] of Object.entries(input.query ?? {})) {
      if (value === undefined || value === null || value === "") continue;
      if (!meta.parameters.some((p) => p.in === "query" && p.name === name))
        throw new HeyError("usage", `Unknown query parameter ${name}`);
      url.searchParams.set(name, String(value));
    }
    return url;
  }
  async execute<K extends OperationName>(
    operation: K,
    input: OperationInput<K>,
    options: RequestOptions = {},
  ): Promise<OperationResponse<K>> {
    return this.request(
      operation,
      this.url(operation, input, options),
      input,
      options,
    ) as Promise<OperationResponse<K>>;
  }
  async request(
    operation: OperationName,
    url: URL,
    input: RawInput,
    options: RequestOptions,
  ): Promise<ApiResponse<unknown>> {
    const meta: Metadata = operationMetadata[operation];
    url = sameOriginURL(url.href, this.base);
    if (this.accountId !== undefined)
      url.searchParams.set("filtered_account_id", String(this.accountId));
    if (meta.bodyRequired && input.body === undefined)
      throw new HeyError("usage", `Missing body for ${operation}`);
    let body = input.body;
    if (body && typeof body === "object" && this.accountId !== undefined) {
      const value = { ...body } as Record<string, unknown>;
      if (meta.actingSender && !value.acting_sender_id)
        value.acting_sender_id = this.senderId();
      if (meta.actingUser && !value.acting_user_id)
        value.acting_user_id = this.userId();
      body = value;
    }
    const encoded = body === undefined ? undefined : stringifyJSON(body);
    const signal = AbortSignal.any([
      AbortSignal.timeout(this.timeout),
      ...(options.signal ? [options.signal] : []),
    ]);
    let refreshed = false;
    const retries = Math.min(
      this.options.maxRetries ?? 3,
      meta.retry?.maxAttempts ?? 3,
    );
    for (let attempt = 0; ; attempt++) {
      signal.throwIfAborted();
      const token = await waitForCredentials(this.credentials.token(), signal);
      if (!token.value)
        throw new HeyError("auth_required", "A HEY access token is required");
      const headers = new Headers({
        Accept: "application/json",
        Authorization: `Bearer ${token.value}`,
        "User-Agent": this.options.userAgent ?? USER_AGENT,
      });
      if (encoded !== undefined)
        headers.set("Content-Type", "application/json");
      const cacheKey = `${createHash("sha256").update(token.value).digest("hex")} ${url.href}`;
      const cacheable = this.options.cache && meta.method === "GET";
      const cached = cacheable ? this.cache.get(cacheKey) : undefined;
      if (cached) headers.set("If-None-Match", cached.headers.get("ETag")!);
      let response: Response;
      signal.throwIfAborted();
      try {
        response = await this.fetch(url, {
          method: meta.method,
          headers,
          body: encoded,
          signal,
          redirect: "manual",
        });
      } catch (cause) {
        // No replay after an ambiguous network failure, even on a mutation with a key.
        networkFailure(cause, signal);
      }
      let text = await readBounded(response, this.maxBytes, signal);
      if (
        response.status === 401 &&
        !refreshed &&
        this.credentials.provider.refresh
      ) {
        refreshed = true;
        signal.throwIfAborted();
        await waitForCredentials(
          this.credentials.refresh(token.generation),
          signal,
        );
        continue;
      }
      if (
        meta.safe &&
        attempt < retries &&
        (meta.retry?.retryOn ?? [429, 503]).includes(response.status)
      ) {
        const retryAfter = response.headers.get("Retry-After");
        let ms = (meta.retry?.baseDelayMs ?? 1000) * 2 ** attempt;
        if (retryAfter !== null) {
          const value = retryAfter.trim();
          if (/^\d+$/.test(value)) {
            const seconds = Number(value);
            if (
              !Number.isFinite(seconds) ||
              seconds > Math.floor(2_147_483_647 / 1000)
            )
              throw responseError(response, undefined);
            ms = seconds * 1000;
          } else {
            const parsed = Date.parse(value) - Date.now();
            if (Number.isFinite(parsed) && parsed >= 0) ms = parsed;
          }
        }
        // Never retry earlier than Retry-After, nor overflow the platform timer.
        if (ms > 2_147_483_647) throw responseError(response, undefined);
        // Node timers may wake slightly early. Use a monotonic deadline so the
        // configured backoff and server Retry-After are minimum delays, not hints.
        const deadline = performance.now() + ms;
        do {
          await delay(
            Math.max(0, Math.ceil(deadline - performance.now())),
            undefined,
            { signal },
          );
        } while (performance.now() < deadline);
        continue;
      }
      if (meta.emptyOn?.includes(response.status))
        return {
          data: undefined,
          status: response.status,
          headers: response.headers,
        };
      let responseHeaders = response.headers;
      let status = response.status;
      if (status === 304 && cached) {
        text = cached.text;
        responseHeaders = new Headers(cached.headers);
        response.headers.forEach((v, k) => responseHeaders.set(k, v));
        status = 200;
      }
      let data: unknown;
      if (text) {
        try {
          data = parseJSON(text);
        } catch (cause) {
          if (status >= 200 && status < 300)
            throw new HeyError(
              "api_error",
              "Invalid JSON response",
              status,
              false,
              "",
              "",
              { cause },
            );
        }
      }
      if (status < 200 || status >= 300) throw responseError(response, data);
      if (cacheable && status === 200) {
        if (
          responseHeaders.has("ETag") &&
          !/no-store/i.test(responseHeaders.get("Cache-Control") ?? "")
        ) {
          if (this.cache.size >= 100)
            this.cache.delete(this.cache.keys().next().value!);
          this.cache.set(cacheKey, {
            text,
            headers: new Headers(responseHeaders),
          });
        } else this.cache.delete(cacheKey);
      }
      if (meta.method !== "GET") this.cache.clear();
      const link = nextLink(responseHeaders.get("Link"));
      const next = link ? sameOriginURL(link, url) : undefined;
      const count = responseHeaders.get("X-Total-Count");
      return {
        data,
        status,
        headers: responseHeaders,
        nextUrl: next?.href,
        nextPage: next?.searchParams.get("page") ?? undefined,
        totalCount:
          count && /^\d+$/.test(count)
            ? (parseJSON(count) as number | bigint)
            : undefined,
      };
    }
  }
  senderId(): number | bigint {
    const senders =
      this.identity?.senders?.filter(
        (s) => String(s.account_id) === String(this.accountId),
      ) ?? [];
    const sender = senders.find((s) => s.default) ?? senders[0];
    if (!sender?.id)
      throw new HeyError("not_found", "No sender for selected account");
    return sender.id;
  }
  userId(): number | bigint {
    const user = this.identity?.all_users?.find(
      (u) => String(u.account_id) === String(this.accountId),
    );
    if (!user?.id)
      throw new HeyError("not_found", "No user for selected account");
    return user.id;
  }
}

/** Full modeled HEY API. Operations and their input/output types are generated. */
export class HeyClient extends GeneratedOperations {
  private http: HttpTransport;
  constructor(options: ClientOptions) {
    const provider =
      typeof options.token === "string"
        ? { getToken: () => options.token as string }
        : options.token;
    const http = new HttpTransport({ ...options }, new Credentials(provider));
    super(http);
    this.http = http;
  }
  get accountId() {
    return this.http.accountId;
  }
  /** Verifies linked-account access once; scope is mail filtering, not authorization. */
  async forAccount(
    accountId: number | bigint,
    options?: RequestOptions,
  ): Promise<HeyClient> {
    assertSafeNumbers(accountId);
    if (
      (typeof accountId !== "bigint" && !Number.isInteger(accountId)) ||
      accountId <= 0
    )
      throw new HeyError("usage", "Account ID must be a positive integer");
    const root = new HttpTransport(this.http.options, this.http.credentials);
    const { data: identity } = await root.execute("GetIdentity", {}, options);
    if (
      !identity?.accounts?.some(
        (a) =>
          String(a.id) === String(accountId) &&
          (a.status === "active" ||
            (a.status === "inactive" &&
              ["work", "domains"].includes(a.purpose ?? ""))),
      )
    )
      throw new HeyError("not_found", "Accessible account not found");
    const client = new HeyClient(this.http.options);
    // Share the provider and refresh single-flight, never mutable account state or cache.
    const scoped = new HttpTransport(this.http.options, this.http.credentials);
    scoped.accountId = accountId;
    scoped.identity = identity;
    client.http = scoped;
    client.transport = scoped;
    return client;
  }
  accountSenderId(): number | bigint {
    return this.http.senderId();
  }
  accountUserId(): number | bigint {
    return this.http.userId();
  }
  /** Yields complete response pages, preserving envelopes and sync bookmarks. */
  async *pages<K extends OperationName>(
    operation: K,
    input: OperationInput<K>,
    options: RequestOptions = {},
  ): AsyncGenerator<OperationResponse<K>> {
    const meta: Metadata = operationMetadata[operation];
    if (meta.pagination?.style !== "link")
      throw new HeyError(
        "usage",
        `${operation} is not Link-paginated; window reads are single requests`,
      );
    let url: URL | undefined = this.http.url(operation, input, options);
    const seen = new Set<string>();
    for (let page = 0; url && page < this.http.maxPages; page++) {
      if (seen.has(url.href))
        throw new HeyError("api_error", "Pagination cycle detected");
      seen.add(url.href);
      const result = (await this.http.request(
        operation,
        url,
        input,
        options,
      )) as OperationResponse<K>;
      yield result;
      url = result.nextUrl ? new URL(result.nextUrl) : undefined;
    }
    if (url) throw new HeyError("api_error", "Pagination exceeded maxPages");
  }
  /** PUT to a self-authenticating Active Storage URL, without HEY auth/account filter. */
  async uploadBytes(
    upload: components["schemas"]["DirectUpload"],
    bytes: Uint8Array,
    options: RequestOptions = {},
  ): Promise<void> {
    const target = upload.direct_upload;
    if (!target?.url)
      throw new HeyError("usage", "Missing direct upload target");
    const url = secureURL(target.url);
    const headers = new Headers(target.headers);
    headers.delete("Authorization");
    headers.delete("Cookie");
    const signal = AbortSignal.any([
      AbortSignal.timeout(this.http.timeout),
      ...(options.signal ? [options.signal] : []),
    ]);
    signal.throwIfAborted();
    let response: Response;
    try {
      response = await this.http.fetch(url, {
        method: "PUT",
        headers,
        body: Buffer.from(bytes),
        redirect: "manual",
        signal,
      });
    } catch (cause) {
      networkFailure(cause, signal);
    }
    await readBounded(response, this.http.maxBytes, signal);
    if (!response.ok) throw responseError(response, undefined);
  }
}
