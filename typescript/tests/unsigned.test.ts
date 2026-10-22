import { describe, it, expect, vi } from "vitest";
import { HeyClient, HeyError, NoCredentials } from "../src/index.js";
import { operationMetadata } from "../src/generated/operations.js";
import { parseJSON, stringifyJSON } from "../src/security.js";
const json = (body: unknown, status = 200) =>
  new Response(stringifyJSON(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
function mock(responses: Response[]) {
  const requests: Request[] = [];
  const fetch = vi.fn<typeof globalThis.fetch>(async (input, init) => {
    requests.push(new Request(input, init));
    const response = responses.shift();
    if (!response) throw new Error("Unexpected request");
    return response;
  });
  return { fetch, requests };
}
const grant = {
  grant_type: "authorization_code",
  client_id: "client",
  install_id: "install",
  code: "code",
  redirect_uri: "heyapp://oauth/callback",
  code_verifier: "verifier",
};
const tokens = {
  access_token: "access",
  token_type: "bearer",
  expires_in: 1209600,
  refresh_token: "refresh",
};
/** A provider that records whether the client asked it for anything. */
function provider() {
  return { getToken: vi.fn(() => "secret"), refresh: vi.fn() };
}
describe("unsigned operations", () => {
  it("trades a code with no Authorization or DPoP, never consulting the provider", async () => {
    const m = mock([json(tokens)]);
    const credentials = provider();
    const result = await new HeyClient({
      token: credentials,
      fetch: m.fetch,
    }).createToken({ body: grant });
    expect(result.data).toEqual(tokens);
    expect(m.requests).toHaveLength(1);
    const [request] = m.requests;
    expect(request!.method).toBe("POST");
    expect(request!.url).toBe("https://app.hey.com/oauth/tokens.json");
    expect(request!.headers.get("Content-Type")).toBe("application/json");
    expect(request!.headers.has("Authorization")).toBe(false);
    expect(request!.headers.has("DPoP")).toBe(false);
    expect(parseJSON(await request!.text())).toEqual(grant);
    expect(credentials.getToken).not.toHaveBeenCalled();
  });
  it("revokes a token in a JSON body, not the query, with no Authorization", async () => {
    const m = mock([json({})]);
    await new HeyClient({ token: "secret", fetch: m.fetch }).revokeToken({
      body: { client_id: "client", token: "access" },
    });
    expect(m.requests).toHaveLength(1);
    const [request] = m.requests;
    expect(request!.method).toBe("DELETE");
    expect(request!.url).toBe("https://app.hey.com/oauth/tokens.json");
    expect(request!.headers.has("Authorization")).toBe(false);
    expect(parseJSON(await request!.text())).toEqual({
      client_id: "client",
      token: "access",
    });
  });
  it.each([
    ["createToken", { body: { ...grant, grant_type: "refresh_token" } }],
    ["revokeToken", { body: { client_id: "client", token: "access" } }],
  ] as const)("answers a 401 on %s as it is, without refreshing or resending", async (name, input) => {
    const m = mock([
      json({ error: "invalid_grant" }, 401),
      json(tokens),
    ]);
    const credentials = provider();
    const client = new HeyClient({ token: credentials, fetch: m.fetch });
    await expect(
      (client[name] as (i: typeof input) => Promise<unknown>)(input),
    ).rejects.toMatchObject({ code: "auth_required", httpStatus: 401 });
    expect(m.requests).toHaveLength(1);
    expect(credentials.refresh).not.toHaveBeenCalled();
    expect(credentials.getToken).not.toHaveBeenCalled();
  });
  it("still refreshes and resends a signed operation's 401", async () => {
    let token = "old";
    const refresh = vi.fn(() => {
      token = "new";
    });
    const m = mock([json({}, 401), json([])]);
    await new HeyClient({
      token: { getToken: () => token, refresh },
      fetch: m.fetch,
    }).listBoxes();
    expect(refresh).toHaveBeenCalledTimes(1);
    expect(m.requests.map((r) => r.headers.get("Authorization"))).toEqual([
      "Bearer old",
      "Bearer new",
    ]);
  });
  it(
    "lets a refresh trade its refresh token on the very client waiting on it",
    async () => {
      // The refresh runs while the 401'd request waits on it; an unsigned request that also
      // waited on the refresh would never be sent, and the refresh would never finish.
      let token = "old";
      const requests: Request[] = [];
      const fetch: typeof globalThis.fetch = async (input, init) => {
        const request = new Request(input, init);
        requests.push(request);
        if (new URL(request.url).pathname === "/oauth/tokens.json")
          return json({ ...tokens, access_token: "new" });
        return request.headers.get("Authorization") === "Bearer new"
          ? json([])
          : json({}, 401);
      };
      const client: HeyClient = new HeyClient({
        token: {
          getToken: () => token,
          refresh: async () => {
            const { data } = await client.createToken({
              body: {
                grant_type: "refresh_token",
                client_id: "client",
                install_id: "install",
                refresh_token: "refresh",
              },
            });
            token = data!.access_token;
          },
        },
        fetch,
      });
      const result = await client.listBoxes();
      expect(result.status).toBe(200);
      expect(
        requests.map((r) => [
          new URL(r.url).pathname,
          r.headers.get("Authorization"),
        ]),
      ).toEqual([
        ["/boxes.json", "Bearer old"],
        ["/oauth/tokens.json", null],
        ["/boxes.json", "Bearer new"],
      ]);
    },
    2000,
  );
  it("does not double the path's .json when the caller asks for JSON", async () => {
    const m = mock([json(tokens)]);
    await new HeyClient({ token: "secret", fetch: m.fetch }).createToken(
      { body: grant },
      { format: "json" },
    );
    expect(new URL(m.requests[0]!.url).pathname).toBe("/oauth/tokens.json");
  });
  it("carries no account filter from a scoped client", async () => {
    const requests: Request[] = [];
    const fetch: typeof globalThis.fetch = async (input, init) => {
      const request = new Request(input, init);
      requests.push(request);
      return new URL(request.url).pathname === "/identity.json"
        ? Response.json({ accounts: [{ id: 42, status: "active" }] })
        : new URL(request.url).pathname === "/oauth/tokens.json"
          ? json(tokens)
          : json([]);
    };
    const scoped = await new HeyClient({ token: "secret", fetch }).forAccount(42);
    await scoped.listBoxes();
    await scoped.createToken({ body: grant });
    const [, boxes, token] = requests;
    expect(new URL(boxes!.url).searchParams.get("filtered_account_id")).toBe("42");
    expect(new URL(token!.url).search).toBe("");
  });
  it("resends a revocation HEY was too busy for, as its retry policy says", async () => {
    const m = mock([json({}, 503), json({})]);
    await new HeyClient({
      token: "secret",
      fetch: m.fetch,
      baseDelayMs: 0,
    }).revokeToken({ body: { client_id: "client", token: "access" } });
    expect(m.requests).toHaveLength(2);
  });
  it("honours the flag on whichever route carries it, not by the operation's name", async () => {
    const meta = operationMetadata.ListBoxes as { unsigned?: true };
    meta.unsigned = true;
    try {
      const m = mock([json({}, 401)]);
      const credentials = provider();
      await expect(
        new HeyClient({ token: credentials, fetch: m.fetch }).listBoxes(),
      ).rejects.toMatchObject({ httpStatus: 401 });
      expect(m.requests).toHaveLength(1);
      expect(m.requests[0]!.headers.has("Authorization")).toBe(false);
      expect(credentials.getToken).not.toHaveBeenCalled();
      expect(credentials.refresh).not.toHaveBeenCalled();
    } finally {
      delete meta.unsigned;
    }
  });
});
describe("no-redirect operations", () => {
  it.each([301, 302, 303, 307, 308])(
    "answers a %s with its status, unread, sent once, nothing to the Location",
    async (status) => {
      let pulled = false;
      const body = new ReadableStream<Uint8Array>(
        {
          pull(controller) {
            pulled = true;
            controller.enqueue(new TextEncoder().encode('{"error":"moved"}'));
            controller.close();
          },
        },
        { highWaterMark: 0 },
      );
      const m = mock([
        new Response(body, {
          status,
          headers: { Location: "https://elsewhere.example/oauth/tokens.json" },
        }),
        json(tokens),
      ]);
      const error = await new HeyClient({ token: "secret", fetch: m.fetch })
        .createToken({ body: grant })
        .catch((e: unknown) => e);
      expect(error).toBeInstanceOf(HeyError);
      expect(error).toMatchObject({ httpStatus: status });
      expect(m.requests).toHaveLength(1);
      expect(m.requests[0]!.redirect).toBe("manual");
      expect(new URL(m.requests[0]!.url).host).toBe("app.hey.com");
      expect(pulled).toBe(false);
    },
  );
  it("does not resend a revocation's redirect", async () => {
    const m = mock([
      new Response(null, { status: 302, headers: { Location: "/elsewhere" } }),
      json({}),
    ]);
    await expect(
      new HeyClient({ token: "secret", fetch: m.fetch }).revokeToken({
        body: { client_id: "client", token: "access" },
      }),
    ).rejects.toMatchObject({ httpStatus: 302 });
    expect(m.requests).toHaveLength(1);
  });
});
describe("NoCredentials", () => {
  it("sends the operations HEY takes unsigned", async () => {
    const m = mock([json(tokens), json({})]);
    const client = new HeyClient({ token: NoCredentials, fetch: m.fetch });
    const { data } = await client.createToken({ body: grant });
    expect(data?.access_token).toBe("access");
    await client.revokeToken({ body: { client_id: "client", token: "access" } });
    expect(m.requests.map((r) => [r.method, new URL(r.url).pathname])).toEqual([
      ["POST", "/oauth/tokens.json"],
      ["DELETE", "/oauth/tokens.json"],
    ]);
    expect(m.requests.every((r) => !r.headers.has("Authorization"))).toBe(true);
  });
  it("refuses a signed operation with auth_required before sending anything", async () => {
    const m = mock([json([])]);
    await expect(
      new HeyClient({ token: NoCredentials, fetch: m.fetch }).listBoxes(),
    ).rejects.toMatchObject({
      code: "auth_required",
      message: expect.stringMatching(/no credentials/),
    });
    expect(m.fetch).not.toHaveBeenCalled();
  });
  it.each([{}, { token: undefined }, { token: null }, { token: {} }])(
    "is the deliberate opt-in: a client given no token at all is refused when built (%j)",
    (options) => {
      expect(() => new HeyClient(options as never)).toThrow(
        expect.objectContaining({ code: "usage" }),
      );
    },
  );
});
describe("token endpoint refusals", () => {
  const refused = (): never => {
    throw new Error("expected a refusal");
  };
  const refuse = (status: number, body: unknown) => {
    const m = mock([json(body, status)]);
    return new HeyClient({ token: NoCredentials, fetch: m.fetch })
      .createToken({ body: grant })
      .then(refused, (e: unknown) => e as HeyError);
  };
  it("carries HEY's sentence in the message, the code and status unchanged", async () => {
    const error = await refuse(400, {
      error: "invalid_request",
      error_description: "install_id doesn't identify this install. Upgrade your client.",
    });
    expect(error).toBeInstanceOf(HeyError);
    expect(error.message).toContain(
      "install_id doesn't identify this install. Upgrade your client.",
    );
    expect(error).toMatchObject({ code: "usage", httpStatus: 400, retryable: false });
  });
  it("tells a dead refresh token from a misconfigured client", async () => {
    const dead = await refuse(401, {
      error: "invalid_grant",
      error_description: "Invalid refresh token.",
    });
    const misconfigured = await refuse(401, {
      error: "invalid_client",
      error_description: "OAuth client is not authorized to revoke this token.",
    });
    for (const error of [dead, misconfigured])
      expect(error).toMatchObject({ code: "auth_required", httpStatus: 401 });
    expect(dead.oauthError).toBe("invalid_grant");
    expect(misconfigured.oauthError).toBe("invalid_client");
    expect(dead.message).toContain("Invalid refresh token.");
    expect(misconfigured.message).toContain(
      "OAuth client is not authorized to revoke this token.",
    );
  });
  it("keeps the code of a refusal that gives no description", async () => {
    const error = await refuse(401, { error: "invalid_grant" });
    expect(error.oauthError).toBe("invalid_grant");
    expect(error.message).toBe("invalid_grant");
  });
  it("names no OAuth code for an operation whose refusals are not RFC 6749's", async () => {
    const m = mock([json({ error: "invalid_grant" }, 401)]);
    const error = await new HeyClient({ token: "secret", fetch: m.fetch })
      .listBoxes()
      .then(refused, (e: unknown) => e as HeyError);
    expect(error).toMatchObject({ code: "auth_required", message: "invalid_grant" });
    expect(error.oauthError).toBeUndefined();
  });
});
