/** HEY OAuth discovery and RFC 7636 PKCE; no Basecamp/Launchpad hosts or grant types. */
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
    scopes?: string[];
  },
): URL {
  const url = secureURL(config.authorization_endpoint);
  for (const [key, value] of Object.entries({
    response_type: "code",
    client_id: input.clientId,
    redirect_uri: input.redirectUri,
    state: input.state,
    code_challenge: input.challenge,
    code_challenge_method: "S256",
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
): Promise<unknown> {
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
  } catch {
    if (response.ok)
      throw new HeyError("api_error", "Invalid OAuth response", response.status);
  }
  if (!response.ok) throw responseError(response, body);
  return body;
}
export async function discover(
  baseUrl = "https://app.hey.com",
  options: OAuthOptions = {},
): Promise<OAuthConfig> {
  const base = secureURL(baseUrl);
  const config = (await request(
    new URL("/.well-known/oauth-authorization-server", base),
    { headers: { Accept: "application/json" } },
    options,
  )) as OAuthConfig;
  if (!config || typeof config !== "object" || Array.isArray(config))
    throw new HeyError("api_error", "Invalid OAuth discovery response");
  const value = config as unknown as Record<string, unknown>;
  for (const field of ["issuer", "authorization_endpoint", "token_endpoint"])
    if (typeof value[field] !== "string" || value[field] === "")
      throw new HeyError("api_error", "Incomplete OAuth discovery response");
  if (
    value.registration_endpoint !== undefined &&
    typeof value.registration_endpoint !== "string"
  )
    throw new HeyError("api_error", "Invalid OAuth discovery response");
  if (
    value.scopes_supported !== undefined &&
    (!Array.isArray(value.scopes_supported) ||
      !value.scopes_supported.every((scope) => typeof scope === "string"))
  )
    throw new HeyError("api_error", "Invalid OAuth discovery response");
  try {
    secureURL(config.issuer);
    secureURL(config.authorization_endpoint);
    secureURL(config.token_endpoint);
    if (config.registration_endpoint !== undefined)
      secureURL(config.registration_endpoint);
  } catch (cause) {
    throw new HeyError("api_error", "Invalid OAuth discovery response", 0, false, "", "", {
      cause,
    });
  }
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
  const token = (await request(
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
  )) as OAuthToken;
  if (
    !token ||
    typeof token !== "object" ||
    Array.isArray(token) ||
    typeof token.access_token !== "string" ||
    token.access_token.length === 0
  )
    throw new HeyError("api_error", "OAuth response has no access token");
  if (
    typeof token.token_type !== "string" ||
    token.token_type.trim().length === 0
  )
    throw new HeyError("api_error", "OAuth response has no token type");
  if (
    (token.refresh_token !== undefined &&
      typeof token.refresh_token !== "string") ||
    (token.scope !== undefined && typeof token.scope !== "string") ||
    (token.expires_in !== undefined &&
      (typeof token.expires_in !== "number" ||
        !Number.isFinite(token.expires_in) ||
        token.expires_in < 0))
  )
    throw new HeyError("api_error", "Invalid OAuth token response");
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
  },
  options: OAuthOptions = {},
): Promise<OAuthToken> {
  if (
    !input.code ||
    !input.redirectUri ||
    !input.clientId ||
    !input.codeVerifier
  )
    throw new HeyError(
      "usage",
      "Code, redirect URI, client ID and PKCE verifier are required",
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
    },
    options,
  );
}
export function refreshToken(
  input: {
    tokenEndpoint: string;
    refreshToken: string;
    clientId?: string;
    clientSecret?: string;
  },
  options: OAuthOptions = {},
): Promise<OAuthToken> {
  if (!input.refreshToken)
    throw new HeyError("usage", "Refresh token is required");
  return tokenRequest(
    input.tokenEndpoint,
    {
      grant_type: "refresh_token",
      refresh_token: input.refreshToken,
      client_id: input.clientId,
      client_secret: input.clientSecret,
    },
    options,
  );
}
