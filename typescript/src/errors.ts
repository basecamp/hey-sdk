export type ErrorCode =
  | "usage"
  | "not_found"
  | "auth_required"
  | "forbidden"
  | "rate_limit"
  | "network"
  | "api_error"
  | "validation"
  | "conflict"
  | "response_too_large";
export class HeyError extends Error {
  readonly name = "HeyError";
  constructor(
    readonly code: ErrorCode,
    message: string,
    readonly httpStatus = 0,
    readonly retryable = false,
    readonly requestId = "",
    readonly hint = "",
    options?: ErrorOptions,
  ) {
    super(message, options);
  }
}
export function responseError(response: Response, body: unknown): HeyError {
  const status = response.status;
  const codes: Record<number, ErrorCode> = {
    400: "usage",
    401: "auth_required",
    403: "forbidden",
    404: "not_found",
    409: "conflict",
    422: "validation",
    429: "rate_limit",
  };
  let message = `HEY returned HTTP ${status}`;
  if (body && typeof body === "object") {
    const b = body as Record<string, unknown>;
    const messages = b.errors ?? b.error ?? b.message;
    if (typeof messages === "string") message = messages;
    else if (Array.isArray(messages))
      message =
        messages.filter((m) => typeof m === "string").join("; ") || message;
  }
  return new HeyError(
    codes[status] ?? "api_error",
    message.slice(0, 500),
    status,
    status === 429 || status >= 500,
    response.headers.get("X-Request-Id") ?? "",
    status === 401
      ? "Re-authenticate with HEY"
      : status === 403
        ? "Re-authenticate with full scope"
        : "",
  );
}
