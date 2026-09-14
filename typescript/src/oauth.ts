/** HEY OAuth, generic metadata discovery, and RFC 7636 PKCE. */
import { randomBytes, createHash } from "node:crypto";
import { HeyError, responseError } from "./errors.js";
import {
  networkFailure,
  secureURL,
  parseJSON,
  readBounded,
} from "./security.js";
export interface OAuthConfig {
  issuer: string;
  authorization_endpoint: string;
  token_endpoint: string;
  registration_endpoint?: string;
  scopes_supported?: string[];
}
export interface OAuthToken {
  access_token: string;
  token_type: string;
  refresh_token?: string;
  expires_in?: number;
  scope?: string;
}
export interface OAuthOptions {
  fetch?: typeof globalThis.fetch;
  signal?: AbortSignal;
}
/** The OAuth endpoints HEY serves. */
export const HEY_OAUTH_CONFIG: Readonly<OAuthConfig> = Object.freeze({
  issuer: "https://app.hey.com",
  authorization_endpoint: "https://app.hey.com/oauth/authorizations/new",
  token_endpoint: "https://app.hey.com/oauth/tokens",
});
interface OAuthResponse {
  body: unknown;
  status: number;
  requestId: string;
}
function invalidOAuthResponse(
  message: string,
  response: OAuthResponse,
  cause?: unknown,
): HeyError {
  return new HeyError(
    "api_error",
    message,
    response.status,
    false,
    response.requestId,
    "",
    cause === undefined ? undefined : { cause },
  );
}
export function generatePKCE() {
  const verifier = randomBytes(32).toString("base64url");
  return {
    verifier,
    challenge: createHash("sha256").update(verifier).digest("base64url"),
  };
}
export function generateState() {
  return randomBytes(32).toString("base64url");
}
export function authorizationURL(
  config: OAuthConfig,
  input: {
    clientId: string;
    redirectUri: string;
    state: string;
    challenge: string;
    installId: string;
    scopes?: string[];
  },
): URL {
  if (
    !input.clientId ||
    !input.redirectUri ||
    !input.state ||
    !input.challenge ||
    !input.installId
  )
    throw new HeyError(
      "usage",
      "Client ID, redirect URI, state, PKCE challenge and installation ID are required",
    );
  const url = secureURL(config.authorization_endpoint);
  for (const [key, value] of Object.entries({
    grant_type: "authorization_code",
    client_id: input.clientId,
    redirect_uri: input.redirectUri,
    state: input.state,
    code_challenge: input.challenge,
    code_challenge_method: "S256",
    install_id: input.installId,
    scope: input.scopes?.join(" "),
  })) {
    if (value !== undefined) url.searchParams.set(key, value);
  }
  return url;
}
async function request(
  url: URL,
  init: RequestInit,
  options: OAuthOptions,
): Promise<OAuthResponse> {
  const signal = AbortSignal.any([
    AbortSignal.timeout(30_000),
    ...(options.signal ? [options.signal] : []),
  ]);
  signal.throwIfAborted();
  let response: Response;
  try {
    response = await (options.fetch ?? globalThis.fetch)(url, {
      ...init,
      redirect: "manual",
      signal,
    });
  } catch (cause) {
    networkFailure(cause, signal);
  }
  const text = await readBounded(response, 1024 * 1024, signal);
  let body: unknown;
  try {
    body = parseJSON(text);
  } catch (cause) {
    if (response.ok)
      throw new HeyError(
        "api_error",
        "Invalid OAuth response",
        response.status,
        false,
        response.headers.get("X-Request-Id") ?? "",
        "",
        { cause },
      );
  }
  if (!response.ok) throw responseError(response, body);
  return {
    body,
    status: response.status,
    requestId: response.headers.get("X-Request-Id") ?? "",
  };
}
export async function discover(
  baseUrl: string,
  options: OAuthOptions = {},
): Promise<OAuthConfig> {
  const base = secureURL(baseUrl);
  if (base.protocol !== "https:" || base.search || base.hash)
    throw new HeyError(
      "usage",
      "OAuth issuer must use HTTPS without a query or fragment",
    );
  const issuerPath =
    base.pathname === "/" ? "" : base.pathname.replace(/\/$/, "");
  const metadataUrl = new URL(
    `${base.origin}/.well-known/oauth-authorization-server${issuerPath}`,
  );
  const response = await request(
    metadataUrl,
    { headers: { Accept: "application/json" } },
    options,
  );
  const config = response.body as OAuthConfig;
  if (!config || typeof config !== "object" || Array.isArray(config))
    throw invalidOAuthResponse("Invalid OAuth discovery response", response);
  const value = config as unknown as Record<string, unknown>;
  for (const field of ["issuer", "authorization_endpoint", "token_endpoint"])
    if (typeof value[field] !== "string" || value[field] === "")
      throw invalidOAuthResponse("Incomplete OAuth discovery response", response);
  if (
    value.registration_endpoint !== undefined &&
    typeof value.registration_endpoint !== "string"
  )
    throw invalidOAuthResponse("Invalid OAuth discovery response", response);
  if (
    value.scopes_supported !== undefined &&
    (!Array.isArray(value.scopes_supported) ||
      !value.scopes_supported.every((scope) => typeof scope === "string"))
  )
    throw invalidOAuthResponse("Invalid OAuth discovery response", response);
  try {
    secureURL(config.issuer);
    secureURL(config.authorization_endpoint);
    secureURL(config.token_endpoint);
    if (config.registration_endpoint !== undefined)
      secureURL(config.registration_endpoint);
  } catch (cause) {
    throw invalidOAuthResponse("Invalid OAuth discovery response", response, cause);
  }
  if (config.issuer !== baseUrl)
    throw invalidOAuthResponse("OAuth discovery issuer does not match", response);
  return config;
}
async function tokenRequest(
  endpoint: string,
  params: Record<string, string | undefined>,
  options: OAuthOptions,
): Promise<OAuthToken> {
  const body = new URLSearchParams();
  for (const [key, value] of Object.entries(params))
    if (value !== undefined) body.set(key, value);
  const response = await request(
    secureURL(endpoint),
    {
      method: "POST",
      headers: {
        Accept: "application/json",
        "Content-Type": "application/x-www-form-urlencoded",
      },
      body,
    },
    options,
  );
  const token = response.body as OAuthToken;
  if (
    !token ||
    typeof token !== "object" ||
    Array.isArray(token) ||
    typeof token.access_token !== "string" ||
    token.access_token.length === 0
  )
    throw invalidOAuthResponse("OAuth response has no access token", response);
  if (!/^[A-Za-z0-9._~+\/-]+=*$/.test(token.access_token))
    throw invalidOAuthResponse("Invalid OAuth access token", response);
  if (
    typeof token.token_type !== "string" ||
    !/^Bearer$/i.test(token.token_type)
  )
    throw invalidOAuthResponse("OAuth response has unsupported token type", response);
  if (
    (token.refresh_token !== undefined &&
      (typeof token.refresh_token !== "string" ||
        !/^[\x20-\x7e]+$/.test(token.refresh_token))) ||
    (token.scope !== undefined &&
      (typeof token.scope !== "string" ||
        !/^[\x21\x23-\x5b\x5d-\x7e]+(?: [\x21\x23-\x5b\x5d-\x7e]+)*$/.test(
          token.scope,
        ))) ||
    (token.expires_in !== undefined &&
      (typeof token.expires_in !== "number" ||
        !Number.isSafeInteger(token.expires_in) ||
        token.expires_in < 0))
  )
    throw invalidOAuthResponse("Invalid OAuth token response", response);
  return token;
}
export function exchangeCode(
  input: {
    tokenEndpoint: string;
    code: string;
    redirectUri: string;
    clientId: string;
    clientSecret?: string;
    codeVerifier: string;
    installId: string;
  },
  options: OAuthOptions = {},
): Promise<OAuthToken> {
  if (
    !input.code ||
    !input.redirectUri ||
    !input.clientId ||
    !input.codeVerifier ||
    !input.installId
  )
    throw new HeyError(
      "usage",
      "Code, redirect URI, client ID, PKCE verifier and installation ID are required",
    );
  return tokenRequest(
    input.tokenEndpoint,
    {
      grant_type: "authorization_code",
      code: input.code,
      redirect_uri: input.redirectUri,
      client_id: input.clientId,
      client_secret: input.clientSecret,
      code_verifier: input.codeVerifier,
      install_id: input.installId,
    },
    options,
  );
}
export function refreshToken(
  input: {
    tokenEndpoint: string;
    refreshToken: string;
    clientId: string;
    clientSecret?: string;
    installId: string;
  },
  options: OAuthOptions = {},
): Promise<OAuthToken> {
  if (!input.refreshToken || !input.clientId || !input.installId)
    throw new HeyError(
      "usage",
      "Refresh token, client ID and installation ID are required",
    );
  return tokenRequest(
    input.tokenEndpoint,
    {
      grant_type: "refresh_token",
      refresh_token: input.refreshToken,
      client_id: input.clientId,
      client_secret: input.clientSecret,
      install_id: input.installId,
    },
    options,
  );
}
