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
