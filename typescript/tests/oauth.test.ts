import { createHash } from "node:crypto";
import { it, expect, vi } from "vitest";
import {
  discover,
  generatePKCE,
  generateState,
  authorizationURL,
  exchangeCode,
  refreshToken,
} from "../src/oauth.js";
const grants = {
  exchangeCode: (options: Parameters<typeof discover>[1]) => exchangeCode({
    tokenEndpoint: "https://app.hey.com/oauth/token",
    code: "code", redirectUri: "http://localhost/callback",
    clientId: "app", codeVerifier: "verifier",
  }, options),
  refreshToken: (options: Parameters<typeof discover>[1]) => refreshToken({
    tokenEndpoint: "https://app.hey.com/oauth/token", refreshToken: "refresh",
  }, options),
};
const entryPoints = { discover: (options: Parameters<typeof discover>[1]) => discover(undefined, options), ...grants };

for (const [name, invoke] of Object.entries(entryPoints)) {
  it.each([401, 403, 429, 500, 503].flatMap(status =>
    ["", "<html>private upstream error</html>", "{"].map(body => ({ status, body })),
  ))(`${name} preserves HTTP error metadata for $status with body '$body'`, async ({ status, body }) => {
    await expect(invoke({ fetch: async () => new Response(body, {
      status, headers: { "X-Request-Id": "oauth-request" },
    }) })).rejects.toMatchObject({
      code: ({ 401: "auth_required", 403: "forbidden", 429: "rate_limit" } as Record<number, string>)[status] ?? "api_error",
      httpStatus: status, retryable: status === 429 || status >= 500,
      requestId: "oauth-request",
      hint: status === 401 ? "Re-authenticate with HEY" : status === 403 ? "Re-authenticate with full scope" : "",
      message: `HEY returned HTTP ${status}`,
    });
  });
  it.each(["", "{"])(`${name} rejects malformed successful JSON '%s'`, async body => {
    await expect(invoke({ fetch: async () => new Response(body) })).rejects.toMatchObject({
      code: "api_error", httpStatus: 200, message: "Invalid OAuth response",
    });
  });
  it(`${name} preserves structured error messages and bounds error bodies`, async () => {
    await expect(invoke({ fetch: async () => Response.json({ error: "denied" }, { status: 401 }) }))
      .rejects.toMatchObject({ code: "auth_required", message: "denied" });
    await expect(invoke({ fetch: async () => new Response("x".repeat(1024 * 1024 + 1), { status: 503 }) }))
      .rejects.toMatchObject({ code: "response_too_large" });
  });
}

for (const [name, invoke] of Object.entries(grants)) {
  it.each([undefined, "", 123, null, false, {}])(`${name} rejects malformed token_type %j`, async token_type => {
    await expect(invoke({ fetch: async () => Response.json({ access_token: "token", token_type }) }))
      .rejects.toMatchObject({ code: "api_error", message: "OAuth response has no token type" });
  });
  it.each(["Bearer", "Custom"])(`${name} accepts non-empty token_type %s`, async token_type => {
    const token = { access_token: "token", token_type, refresh_token: "rotated" };
    await expect(invoke({ fetch: async () => Response.json(token) })).resolves.toEqual(token);
  });
}

it("creates RFC 7636 PKCE and unpredictable state without browser claims", () => {
  const p = generatePKCE();
  expect(p.verifier).toHaveLength(43);
  expect(p.challenge).toBe(
    createHash("sha256").update(p.verifier).digest("base64url"),
  );
  expect(generateState()).not.toBe(generateState());
});
it("discovers HEY endpoints, builds authorization URL and exchanges/refreshes standard form grants", async () => {
  const requests: Request[] = [];
  const config = {
    issuer: "https://app.hey.com",
    authorization_endpoint: "https://app.hey.com/oauth/authorize",
    token_endpoint: "https://app.hey.com/oauth/token",
  };
  const fetch: typeof globalThis.fetch = async (input, init) => {
    const r = new Request(input, init);
    requests.push(r);
    return Response.json(
      r.method === "GET"
        ? config
        : {
            access_token: "new",
            token_type: "Bearer",
            refresh_token: "refresh",
          },
    );
  };
  expect(await discover(undefined, { fetch })).toEqual(config);
  expect(requests[0]!.url).toBe(
    "https://app.hey.com/.well-known/oauth-authorization-server",
  );
  const url = authorizationURL(config, {
    clientId: "app",
    redirectUri: "http://localhost/callback",
    state: "state",
    challenge: "challenge",
    scopes: ["read", "write"],
  });
  expect(url.searchParams.get("code_challenge_method")).toBe("S256");
  expect(url.searchParams.get("scope")).toBe("read write");
  await exchangeCode(
    {
      tokenEndpoint: config.token_endpoint,
      code: "a+b",
      redirectUri: "http://localhost/callback",
      clientId: "app",
      codeVerifier: "verifier",
    },
    { fetch },
  );
  const form = new URLSearchParams(await requests[1]!.text());
  expect(form.get("grant_type")).toBe("authorization_code");
  expect(form.get("code")).toBe("a+b");
  expect(form.get("code_verifier")).toBe("verifier");
  expect(requests[1]!.redirect).toBe("manual");
  await refreshToken(
    {
      tokenEndpoint: config.token_endpoint,
      refreshToken: "refresh",
      clientId: "app",
    },
    { fetch },
  );
  expect(new URLSearchParams(await requests[2]!.text()).get("grant_type")).toBe(
    "refresh_token",
  );
});
it("refuses insecure discovery/token targets and token redirects, limits bodies", async () => {
  const fetch = vi.fn(async () => Response.json({}));
  await expect(
    discover("http://evil.example", { fetch }),
  ).rejects.toMatchObject({ code: "usage" });
  expect(fetch).not.toHaveBeenCalled();
  await expect(
    refreshToken(
      { tokenEndpoint: "http://evil.example", refreshToken: "r" },
      { fetch },
    ),
  ).rejects.toMatchObject({ code: "usage" });
  await expect(
    refreshToken(
      { tokenEndpoint: "https://app.hey.com/oauth/token", refreshToken: "r" },
      {
        fetch: async () =>
          Response.json(
            {},
            { status: 302, headers: { Location: "https://evil.example" } },
          ),
      },
    ),
  ).rejects.toMatchObject({ httpStatus: 302 });
  await expect(
    discover(undefined, {
      fetch: async () => new Response("x".repeat(1024 * 1024 + 1)),
    }),
  ).rejects.toMatchObject({ code: "response_too_large" });
});
