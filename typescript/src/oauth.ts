/** HEY OAuth discovery and RFC 7636 PKCE; no Basecamp/Launchpad hosts or grant types. */
import { randomBytes, createHash } from "node:crypto";
import { HeyError, responseError } from "./errors.js";
import { secureURL, parseJSON, readBounded } from "./security.js";
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
  const response = await (options.fetch ?? globalThis.fetch)(url, {
    ...init,
    redirect: "manual",
    signal: AbortSignal.any([
      AbortSignal.timeout(30_000),
      ...(options.signal ? [options.signal] : []),
    ]),
  });
  const text = await readBounded(response, 1024 * 1024);
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
  if (
    !config?.issuer ||
    !config.authorization_endpoint ||
    !config.token_endpoint
  )
    throw new HeyError("api_error", "Incomplete OAuth discovery response");
  secureURL(config.issuer);
  secureURL(config.authorization_endpoint);
  secureURL(config.token_endpoint);
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
  if (!token?.access_token || typeof token.access_token !== "string")
    throw new HeyError("api_error", "OAuth response has no access token");
  if (!token.token_type || typeof token.token_type !== "string")
    throw new HeyError("api_error", "OAuth response has no token type");
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
