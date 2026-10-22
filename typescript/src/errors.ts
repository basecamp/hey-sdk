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
/**
 * One reason HEY gave for refusing what was sent (422): the field it objected to, when it
 * named one, and what it said. HEY answers a 422 with `{"errors": ["Subject can't be blank"]}`,
 * the model's own full messages, which name no field apart from the message, so `field` is
 * absent for those; an entry that is an object, `{"field": "subject", "message": "can't be
 * blank"}` (`attribute` is read as well), carries both.
 */
export interface FieldError {
  readonly field?: string;
  readonly message: string;
}
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
    /**
     * Each reason HEY gave for a 422, in the order it gave them: the entries of the body's
     * `errors` list, or its single `error` or `message` when the list is absent or gives no
     * usable reason (so `{"errors":[],"message":"Invalid"}` gives one). The message joins the
     * same reasons.
     * Empty for any other error.
     */
    readonly fieldErrors: readonly FieldError[] = [],
    /**
     * The RFC 6749 error code an OAuth token endpoint refused with (`invalid_grant`,
     * `invalid_client`, ...), on an operation whose refusals take that shape. It is what a
     * caller acts on: a 401 `invalid_grant` is a dead refresh token, a 401 `invalid_client`
     * a misconfigured client. Absent for any other error.
     */
    readonly oauthError?: string,
  ) {
    super(message, options);
  }
}
const text = (value: unknown): value is string =>
  typeof value === "string" && value.trim() !== "";
/** One entry of an `errors` list: a message, or an object naming the field and the message. */
function fieldError(entry: unknown): FieldError | undefined {
  if (text(entry)) return { message: entry };
  if (!entry || typeof entry !== "object") return undefined;
  const e = entry as Record<string, unknown>;
  if (!text(e.message)) return undefined;
  const field = text(e.field) ? e.field : text(e.attribute) ? e.attribute : undefined;
  return field === undefined ? { message: e.message } : { field, message: e.message };
}
/**
 * The reasons a failure body gives: the `errors` list, or RFC 6749's `error` with its
 * `error_description`, or its single `error` or `message`.
 */
function reasonsIn(body: unknown): FieldError[] {
  if (!body || typeof body !== "object") return [];
  const b = body as Record<string, unknown>;
  if (Array.isArray(b.errors)) {
    const listed = b.errors.flatMap((entry) => fieldError(entry) ?? []);
    if (listed.length > 0) return listed;
  }
  // The description is the sentence for a person ("install_id doesn't identify this
  // install. Upgrade your client."); the code alone would leave them nothing to act on.
  if (text(b.error_description))
    return [
      {
        message: text(b.error)
          ? `${b.error}: ${b.error_description}`
          : b.error_description,
      },
    ];
  const single = [b.error, b.message].find(text);
  return single === undefined ? [] : [{ message: single }];
}
const reasonText = (reason: FieldError) =>
  reason.field === undefined ? reason.message : `${reason.field}: ${reason.message}`;
/**
 * `oauth` says the operation refuses in RFC 6749's shape, so the body's `error` is an OAuth
 * code worth keeping apart from the message; elsewhere HEY's `{"error"}` is only a sentence.
 */
export function responseError(
  response: Response,
  body: unknown,
  oauth = false,
): HeyError {
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
  const reasons = reasonsIn(body);
  const message =
    reasons.map(reasonText).join("; ") || `HEY returned HTTP ${status}`;
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
    undefined,
    status === 422 ? reasons : [],
    oauth && body && typeof body === "object"
      ? [(body as Record<string, unknown>).error].find(text)
      : undefined,
  );
}
