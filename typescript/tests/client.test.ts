import { describe, it, expect, vi } from "vitest";
import { HeyClient, HeyError } from "../src/index.js";
import { nextLink, parseJSON, stringifyJSON } from "../src/security.js";
import { VERSION, API_VERSION } from "../src/version.js";
function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((r) => {
    resolve = r;
  });
  return { promise, resolve };
}
const json = (
  body: unknown,
  status = 200,
  headers: Record<string, string> = {},
) =>
  new Response(stringifyJSON(body), {
    status,
    headers: { "Content-Type": "application/json", ...headers },
  });
function mock(responses: Response[]) {
  const requests: Request[] = [];
  const fetch: typeof globalThis.fetch = async (input, init) => {
    requests.push(new Request(input, init));
    const response = responses.shift();
    if (!response) throw new Error("Unexpected request");
    return response;
  };
  return { fetch, requests };
}
describe("HEY transport", () => {
  it("preserves int64 path/body/response, decimals, nulls and absent fields", async () => {
    const m = mock([
      json({ id: 9007199254740993n, position: 1.5, description: null }),
    ]);
    const result = await new HeyClient({
      token: "secret",
      fetch: m.fetch,
    }).getBox({ path: { boxId: 9007199254740993n } });
    expect(result.data).toEqual({
      id: 9007199254740993n,
      position: 1.5,
      description: null,
    });
    expect(m.requests[0]!.url).toContain("/boxes/9007199254740993");
    expect(parseJSON(stringifyJSON({ id: 9007199254740993n }))).toEqual({
      id: 9007199254740993n,
    });
    expect(result.data).not.toHaveProperty("name");
  });
  it("rejects unsafe input numbers before auth/network", async () => {
    const fetch = vi.fn();
    const c = new HeyClient({ token: "secret", fetch });
    await expect(
      c.getBox({ path: { boxId: 9007199254740992 } }),
    ).rejects.toMatchObject({ code: "usage" });
    await expect(
      c.movePostings({ body: { posting_ids: [9007199254740992], box_id: 1 } }),
    ).rejects.toMatchObject({ code: "usage" });
    expect(fetch).not.toHaveBeenCalled();
  });
  it.each(["..", ".", "../identity", "a/b", "%2e%2e", "a?b", "a\\b"])(
    "refuses malicious path label %s",
    async (day) => {
      const fetch = vi.fn();
      const c = new HeyClient({ token: "secret", fetch });
      await expect(c.getCalendarDay({ path: { day } })).rejects.toMatchObject({
        code: "usage",
      });
      expect(fetch).not.toHaveBeenCalled();
    },
  );
  it.each([
    "http://evil.example",
    "https://user:pass@app.hey.com",
    "file:///tmp/x",
    "https://app.hey.com/#secret",
    "https://app.hey.com/prefix",
  ])("refuses unsafe base %s", (baseUrl) => {
    expect(() => new HeyClient({ token: "secret", baseUrl })).toThrow(HeyError);
  });
  it("uses HEY authorization, Accept and versioned User-Agent", async () => {
    const m = mock([json([])]);
    await new HeyClient({ token: "secret", fetch: m.fetch }).listBoxes();
    expect(m.requests[0]!.headers.get("Authorization")).toBe("Bearer secret");
    expect(m.requests[0]!.headers.get("Accept")).toBe("application/json");
    expect(m.requests[0]!.headers.get("User-Agent")).toBe(
      `hey-sdk-typescript/${VERSION} (API ${API_VERSION})`,
    );
  });
  it.each([301, 302, 303, 307, 308])(
    "never follows %s redirects with credentials or bodies",
    async (status) => {
      const m = mock([
        new Response(null, {
          status,
          headers: { Location: "https://evil.example/collect" },
        }),
      ]);
      await expect(
        new HeyClient({ token: "secret", fetch: m.fetch }).createMessage({
          body: {
            acting_sender_id: 0,
            message: { subject: "x", content: "x" },
          },
        }),
      ).rejects.toMatchObject({ httpStatus: status });
      expect(m.requests).toHaveLength(1);
      expect(m.requests[0]!.redirect).toBe("manual");
    },
  );
  it("bounds success and error bodies including decompressed streamed chunks", async () => {
    for (const status of [200, 503]) {
      let cancelled = false;
      const response = new Response(
        new ReadableStream({
          pull(c) {
            c.enqueue(new Uint8Array(100));
          },
          cancel() {
            cancelled = true;
          },
        }),
        { status },
      );
      const m = mock([response]);
      await expect(
        new HeyClient({
          token: "secret",
          fetch: m.fetch,
          maxResponseBodyBytes: 50,
        }).listBoxes(),
      ).rejects.toMatchObject({
        code: "response_too_large",
        httpStatus: status,
        retryable: false,
      });
      expect(cancelled).toBe(true);
      expect(m.requests).toHaveLength(1);
    }
  });
  it("returns undefined only for annotated 404, and empty 204 success", async () => {
    const m = mock([
      json({}, 404),
      json({}, 404),
      new Response(null, { status: 204 }),
    ]);
    const c = new HeyClient({ token: "secret", fetch: m.fetch });
    expect((await c.getOngoingTimeTrack()).data).toBeUndefined();
    await expect(c.getBox({ path: { boxId: 1 } })).rejects.toMatchObject({
      code: "not_found",
    });
    expect((await c.startTimeTrack()).data).toBeUndefined();
  });
  it("preserves 409 and 422 messages and request IDs; bounds error text", async () => {
    const m = mock([
      json({ errors: ["a", "b"] }, 422, { "X-Request-Id": "r" }),
      json({ error: "x".repeat(900) }, 409),
    ]);
    const c = new HeyClient({ token: "secret", fetch: m.fetch });
    await expect(c.listBoxes()).rejects.toMatchObject({
      code: "validation",
      message: "a; b",
      requestId: "r",
    });
    await expect(c.listBoxes()).rejects.toMatchObject({
      code: "conflict",
      message: "x".repeat(500),
    });
  });
  it("rejects malformed successful JSON, does not echo HTML errors", async () => {
    const m = mock([
      new Response("<html>secret</html>"),
      new Response("<html>secret</html>", { status: 403 }),
    ]);
    const c = new HeyClient({ token: "secret", fetch: m.fetch });
    await expect(c.listBoxes()).rejects.toMatchObject({
      message: "Invalid JSON response",
    });
    await expect(c.listBoxes()).rejects.toMatchObject({
      code: "forbidden",
      message: "HEY returned HTTP 403",
    });
  });
  it("does not replay non-idempotent POST or explicitly unsafe PUT on 429/503", async () => {
    for (const status of [429, 503]) {
      const m = mock([json({}, status), json({}, status)]);
      const c = new HeyClient({ token: "secret", fetch: m.fetch });
      await expect(
        c.createMessage({
          body: {
            acting_sender_id: 0,
            message: { subject: "x", content: "y" },
          },
        }),
      ).rejects.toMatchObject({ httpStatus: status });
      await expect(
        c.updateMessage({
          path: { messageId: 1 },
          body: {
            acting_sender_id: 0,
            message: { subject: "x", content: "y" },
          },
        }),
      ).rejects.toMatchObject({ httpStatus: status });
      expect(m.requests).toHaveLength(2);
    }
  });
  it("retries safe operations with replayed query and zero Retry-After", async () => {
    const m = mock([json({}, 503, { "Retry-After": "0" }), json([])]);
    const c = new HeyClient({ token: "secret", fetch: m.fetch });
    await c.listContacts({ query: { page: "cursor+with/slash" } });
    expect(m.requests).toHaveLength(2);
    expect(m.requests[0]!.url).toBe(m.requests[1]!.url);
  });
  it("honors maxRetries=0 and never overflows huge Retry-After", async () => {
    for (const options of [{ maxRetries: 0 }, {}]) {
      const m = mock([json({}, 429, { "Retry-After": "999999999999" })]);
      await expect(
        new HeyClient({
          token: "secret",
          fetch: m.fetch,
          ...options,
        }).listBoxes(),
      ).rejects.toMatchObject({ code: "rate_limit" });
      expect(m.requests).toHaveLength(1);
    }
  });
  it("rejects non-finite Retry-After seconds without an early resend", async () => {
    const m = mock([
      json({}, 429, { "Retry-After": "9".repeat(400) }),
      json([]),
    ]);
    await expect(
      new HeyClient({ token: "secret", fetch: m.fetch }).listBoxes(),
    ).rejects.toMatchObject({ code: "rate_limit", httpStatus: 429 });
    expect(m.requests).toHaveLength(1);
  });
  it("normalizes generated Fetch and response-stream failures without retrying", async () => {
    const failures = [
      vi.fn(async () => {
        throw new Error("secret socket detail");
      }),
      vi.fn(async () =>
        new Response(
          new ReadableStream({
            start(controller) {
              controller.error(new Error("secret stream detail"));
            },
          }),
        ),
      ),
    ];
    for (const fetch of failures) {
      const error = await new HeyClient({ token: "secret", fetch })
        .listBoxes()
        .catch((value: unknown) => value);
      expect(error).toMatchObject({
        code: "network",
        message: "Network request failed",
        cause: expect.any(Error),
      });
      expect(fetch).toHaveBeenCalledTimes(1);
    }
  });
  it("does not replay ambiguous network failures", async () => {
    const fetch = vi.fn(async () => {
      throw new Error("socket closed");
    });
    await expect(
      new HeyClient({ token: "secret", fetch }).createMessage({
        body: { acting_sender_id: 0, message: { subject: "x", content: "x" } },
      }),
    ).rejects.toMatchObject({ code: "network" });
    expect(fetch).toHaveBeenCalledTimes(1);
  });
  it("charges a refresh resend to the remaining retry budget", async () => {
    const m = mock([json({}, 401), json({}, 503), json([])]);
    await expect(
      new HeyClient({
        token: { getToken: () => "token", refresh: vi.fn() },
        fetch: m.fetch,
        maxRetries: 1,
      }).listBoxes(),
    ).rejects.toMatchObject({ httpStatus: 503 });
    expect(m.requests).toHaveLength(2);
  });
  it("refreshes once on a definite 401 and replays exact mutation body", async () => {
    let token = "old";
    const refresh = vi.fn(() => {
      token = "new";
    });
    const m = mock([json({}, 401), json({}, 401)]);
    await expect(
      new HeyClient({
        token: { getToken: () => token, refresh },
        fetch: m.fetch,
      }).createMessage({
        body: { acting_sender_id: 0, message: { subject: "x", content: "x" } },
      }),
    ).rejects.toMatchObject({ code: "auth_required" });
    expect(refresh).toHaveBeenCalledTimes(1);
    expect(m.requests[1]!.headers.get("Authorization")).toBe("Bearer new");
    expect(await m.requests[0]!.text()).toBe(await m.requests[1]!.text());
  });
  it("coalesces concurrent refreshes and propagates failures", async () => {
    let token = "old";
    const refresh = vi.fn(async () => {
      await Promise.resolve();
      token = "new";
    });
    const fetch: typeof globalThis.fetch = async (_url, init) =>
      new Headers(init?.headers).get("Authorization") === "Bearer old"
        ? json({}, 401)
        : json([]);
    const c = new HeyClient({
      token: { getToken: () => token, refresh },
      fetch,
    });
    await Promise.all([c.listBoxes(), c.listBoxes()]);
    expect(refresh).toHaveBeenCalledTimes(1);
    const failed = new HeyClient({ token: { getToken: () => "", refresh } });
    await expect(failed.listBoxes()).rejects.toMatchObject({
      code: "auth_required",
    });
  });
  it.each(["timeout", "caller abort"])(
    "settles a hanging getToken on %s without sending",
    async (cancellation) => {
      const started = deferred<void>();
      const controller = new AbortController();
      const fetch = vi.fn();
      const c = new HeyClient({
        token: {
          getToken: () => {
            started.resolve();
            return new Promise<string>(() => {});
          },
        },
        fetch,
        timeoutMs: cancellation === "timeout" ? 10 : 30_000,
      });
      const request = c.listBoxes({}, { signal: controller.signal });
      const rejected = expect(request).rejects.toMatchObject({
        name: cancellation === "timeout" ? "TimeoutError" : "AbortError",
      });
      await started.promise;
      if (cancellation === "caller abort") controller.abort();
      await rejected;
      expect(fetch).not.toHaveBeenCalled();
    },
    1000,
  );
  it.each(["timeout", "caller abort"])(
    "settles a hanging refresh on %s without replaying",
    async (cancellation) => {
      const started = deferred<void>();
      const controller = new AbortController();
      const refresh = vi.fn(() => {
        started.resolve();
        return new Promise<void>(() => {});
      });
      const m = mock([json({}, 401)]);
      const c = new HeyClient({
        token: { getToken: () => "old", refresh },
        fetch: m.fetch,
        timeoutMs: cancellation === "timeout" ? 10 : 30_000,
      });
      const request = c.listBoxes({}, { signal: controller.signal });
      const rejected = expect(request).rejects.toMatchObject({
        name: cancellation === "timeout" ? "TimeoutError" : "AbortError",
      });
      await started.promise;
      if (cancellation === "caller abort") controller.abort();
      await rejected;
      expect(refresh).toHaveBeenCalledTimes(1);
      expect(m.requests).toHaveLength(1);
    },
    1000,
  );
  it("cancels one refresh waiter without cancelling another or replaying the cancelled request", async () => {
    const renewal = deferred<void>();
    let token = "old";
    const refresh = vi.fn(async () => {
      await renewal.promise;
      token = "new";
    });
    const fetch = vi.fn<typeof globalThis.fetch>(async (_url, init) =>
      new Headers(init?.headers).get("Authorization") === "Bearer old"
        ? json({}, 401)
        : json([]),
    );
    const c = new HeyClient({
      token: { getToken: () => token, refresh },
      fetch,
    });
    const controller = new AbortController();
    const cancelled = c.listBoxes({}, { signal: controller.signal });
    const rejected = expect(cancelled).rejects.toMatchObject({
      name: "AbortError",
    });
    const other = c.listBoxes();
    await new Promise((r) => setImmediate(r));
    expect(fetch).toHaveBeenCalledTimes(2);
    expect(refresh).toHaveBeenCalledTimes(1);
    controller.abort();
    await rejected;
    renewal.resolve();
    expect((await other).data).toEqual([]);
    expect(refresh).toHaveBeenCalledTimes(1);
    expect(fetch).toHaveBeenCalledTimes(3);
  }, 1000);
  it("does not dispatch when cancellation coincides with token acquisition", async () => {
    const controller = new AbortController();
    const fetch = vi.fn();
    const c = new HeyClient({
      token: {
        getToken: async () => {
          controller.abort();
          return "token";
        },
      },
      fetch,
    });
    await expect(
      c.listBoxes({}, { signal: controller.signal }),
    ).rejects.toMatchObject({ name: "AbortError" });
    expect(fetch).not.toHaveBeenCalled();
  });
  it("does not rotate again when an old async token read resolves after a completed refresh", async () => {
    const oldRead = deferred<string>();
    let token = "old";
    const getToken = vi.fn((): string | Promise<string> => token)
      .mockImplementationOnce(() => oldRead.promise);
    const refresh = vi.fn(() => {
      token = "new";
    });
    const authorization: string[] = [];
    const fetch: typeof globalThis.fetch = async (_url, init) => {
      const value = new Headers(init?.headers).get("Authorization")!;
      authorization.push(value);
      return value === "Bearer old" ? json({}, 401) : json([]);
    };
    const c = new HeyClient({ token: { getToken, refresh }, fetch });
    const delayed = c.listBoxes();
    expect((await c.listBoxes()).data).toEqual([]);
    expect(refresh).toHaveBeenCalledTimes(1);
    oldRead.resolve("old");
    expect((await delayed).data).toEqual([]);
    expect(authorization).toEqual([
      "Bearer old", "Bearer new", "Bearer old", "Bearer new",
    ]);
    expect(refresh).toHaveBeenCalledTimes(1);
  });
  it("aborts before sending or while waiting for a retry", async () => {
    const controller = new AbortController();
    controller.abort(new Error("cancelled"));
    const m = mock([json({}, 503)]);
    const c = new HeyClient({ token: "secret", fetch: m.fetch });
    await expect(
      c.listBoxes({}, { signal: controller.signal }),
    ).rejects.toThrow("cancelled");
    expect(m.requests).toHaveLength(0);
    const later = new AbortController();
    const request = c.listBoxes({}, { signal: later.signal });
    await new Promise((r) => setImmediate(r));
    later.abort();
    await expect(request).rejects.toThrow();
    expect(m.requests).toHaveLength(1);
  });
});
describe("HEY pagination", () => {
  it("parses complex Link relations without mistaking quoted parameter text for rel", () => {
    expect(
      nextLink(
        '<https://app.hey.com/contacts.json?x=1,2>; rel="prev NEXT"; title="a,b", </old>; rel=prev',
      ),
    ).toBe("https://app.hey.com/contacts.json?x=1,2");
    expect(
      nextLink(
        '</fake>; title="not a relation; rel=next; \\"still quoted\\"", </real?page=2>; title="ok; still ok"; rel="next"',
      ),
    ).toBe("/real?page=2");
    expect(
      nextLink('</contacts.json?page=2>; rel=next   ; title=page'),
    ).toBe("/contacts.json?page=2");
  });
  it("follows the real next link after fake rel text in a quoted parameter", async () => {
    const m = mock([
      json([], 200, {
        Link: '</fake>; title="ignore; rel=next; \\"quoted\\"", </contacts.json?page=2>; rel=next   ; title=page',
      }),
      json([]),
    ]);
    const pages = new HeyClient({ token: "secret", fetch: m.fetch }).pages(
      "ListContacts",
      {},
    );
    await pages.next();
    await pages.next();
    expect(new URL(m.requests[1]!.url).searchParams.get("page")).toBe("2");
  });
  it("follows full relative URLs and preserves envelopes instead of treating bookmarks as pages", async () => {
    const m = mock([
      json(
        {
          postings: [{ id: 1 }],
          next_history_url: "https://evil.example/sync",
        },
        200,
        { Link: '<?page=opaque%2B>; rel="next"' },
      ),
      json({ postings: [{ id: 2 }] }),
    ]);
    const c = new HeyClient({ token: "secret", fetch: m.fetch });
    const data = [];
    for await (const page of c.pages("GetContact", { path: { contactId: 1 } }))
      data.push(page.data);
    expect(data).toHaveLength(2);
    expect(new URL(m.requests[1]!.url).searchParams.get("page")).toBe(
      "opaque+",
    );
  });
  it("stops a changes-feed page sequence at its terminal sync cursor", async () => {
    const m = mock([
      json({ added: [{ id: 1 }] }, 200, {
        Link: "<?since=start&page=page-2>; rel=next",
      }),
      json({ added: [{ id: 2 }] }, 200, {
        Link: "<?since=next-sync>; rel=next",
      }),
    ]);
    const pages = [];
    for await (const page of new HeyClient({
      token: "secret",
      fetch: m.fetch,
    }).pages("GetBoxPostingChanges", {
      path: { boxId: 1 },
      query: { since: "start" },
    }))
      pages.push(page);
    expect(m.requests).toHaveLength(2);
    expect(pages).toHaveLength(2);
    expect(new URL(pages[1]!.nextUrl!).searchParams.get("since")).toBe(
      "next-sync",
    );
  });
  it.each([
    "https://evil.example/x",
    "//evil.example/x",
    "http://app.hey.com/x",
    "https://secret@app.hey.com/x",
  ])("rejects unsafe next link %s before second request", async (link) => {
    const m = mock([json([], 200, { Link: `<${link}>; rel=next` })]);
    await expect(
      new HeyClient({ token: "secret", fetch: m.fetch }).listContacts(),
    ).rejects.toMatchObject({ code: "usage" });
    expect(m.requests).toHaveLength(1);
  });
  it("caps pages explicitly, detects cycles and refuses window pagination", async () => {
    const m = mock([json([], 200, { Link: "<?page=2>; rel=next" })]);
    const c = new HeyClient({ token: "secret", fetch: m.fetch, maxPages: 1 });
    const iterator = c.pages("ListContacts", {});
    await iterator.next();
    await expect(iterator.next()).rejects.toThrow("maxPages");
    const cycle = mock([json([], 200, { Link: "</contacts.json>; rel=next" })]);
    const it = new HeyClient({ token: "secret", fetch: cycle.fetch }).pages(
      "ListContacts",
      {},
    );
    await it.next();
    await expect(it.next()).rejects.toThrow("cycle");
    await expect(
      c.pages("GetCalendarRecordings", { path: { calendarId: 1 } }).next(),
    ).rejects.toThrow("window");
  });
});
it("rejects unsafe exponential integer encodings rather than rounding IDs", async () => {
  const m = mock([new Response('{"id":9007199254740993e0}')]);
  await expect(
    new HeyClient({ token: "x", fetch: m.fetch }).getBox({
      path: { boxId: 1 },
    }),
  ).rejects.toMatchObject({ message: "Invalid JSON response" });
});
it("accepts the reliable timer maximum and rejects larger timeouts before credentials", async () => {
  const token = vi.fn(() => "token");
  expect(
    () => new HeyClient({ token: { getToken: token }, timeoutMs: 2_147_483_647 }),
  ).not.toThrow();
  expect(
    () => new HeyClient({ token: { getToken: token }, timeoutMs: 2_147_483_648 }),
  ).toThrow(/timer maximum/);
  expect(token).not.toHaveBeenCalled();
});
it("normalizes upload Fetch and response-stream failures without leaking details", async () => {
  const upload = {
    signed_id: "signed",
    attachable_sgid: "attachable",
    direct_upload: { url: "https://uploads.example/file" },
  };
  for (const fetch of [
    vi.fn(async () => {
      throw new Error("private upload socket");
    }),
    vi.fn(async () =>
      new Response(
        new ReadableStream({
          start(controller) {
            controller.error(new Error("private upload stream"));
          },
        }),
      ),
    ),
  ]) {
    const error = await new HeyClient({ token: "x", fetch })
      .uploadBytes(upload, new Uint8Array([1]))
      .catch((value: unknown) => value);
    expect(error).toMatchObject({
      code: "network",
      message: "Network request failed",
      cause: expect.any(Error),
    });
    expect(fetch).toHaveBeenCalledTimes(1);
  }
});
it("applies operation deadlines to Fetch", async () => {
  const fetch: typeof globalThis.fetch = async (_url, init) =>
    new Promise((_resolve, reject) => {
      init!.signal!.addEventListener(
        "abort",
        () => reject(init!.signal!.reason),
        { once: true },
      );
    });
  await expect(
    new HeyClient({ token: "x", fetch, timeoutMs: 5 }).listBoxes(),
  ).rejects.toMatchObject({ name: "TimeoutError" });
});
