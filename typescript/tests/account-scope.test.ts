import { it, expect, vi } from "vitest";
import { HeyClient } from "../src/index.js";
const identity = {
  accounts: [
    { id: 42, status: "active" },
    { id: 43, status: "inactive", purpose: "work" },
    { id: 44, status: "inactive", purpose: "personal" },
  ],
  senders: [
    { id: 9, account_id: 42 },
    { id: 10, account_id: 42, default: true },
  ],
  all_users: [{ id: 11, account_id: 42 }],
};
function server() {
  const requests: Request[] = [];
  let next = false;
  const fetch: typeof globalThis.fetch = async (input, init) => {
    const request = new Request(input, init);
    requests.push(request);
    if (new URL(request.url).pathname === "/identity.json")
      return Response.json(identity);
    if (
      new URL(request.url).pathname === "/contacts.json" &&
      request.method === "GET" &&
      !next
    ) {
      next = true;
      return Response.json([], {
        headers: { Link: "<?page=next&filtered_account_id=999>; rel=next" },
      });
    }
    return Response.json([]);
  };
  return { fetch, requests };
}
it("derives immutable linked clients, verifies access, replaces pagination filters and selects acting IDs", async () => {
  const m = server();
  const root = new HeyClient({ token: "a", fetch: m.fetch });
  const c = await root.forAccount(42);
  expect(root.accountId).toBeUndefined();
  expect(c.accountId).toBe(42);
  expect(c.accountSenderId()).toBe(10);
  expect(c.accountUserId()).toBe(11);
  await c.createMessage({
    body: {
      acting_sender_id: 0,
      message: { subject: "hello", content: "world" },
    },
  });
  await c.createContact({
    body: { contact: { name: "a", email_address: "a@example.com" } },
  });
  expect(JSON.parse(await m.requests[1]!.text()).acting_sender_id).toBe(10);
  expect(JSON.parse(await m.requests[2]!.text()).acting_user_id).toBe(11);
  for await (const _page of c.pages("ListContacts", {})) {
    /* exhaust the SDK iterator */
  }
  for (const request of m.requests.slice(1))
    expect(
      new URL(request.url).searchParams.getAll("filtered_account_id"),
    ).toEqual(["42"]);
  await root.listBoxes();
  expect(new URL(m.requests.at(-1)!.url).search).toBe("");
  await expect(root.forAccount(44)).rejects.toMatchObject({
    code: "not_found",
  });
  await expect(root.forAccount(999)).rejects.toMatchObject({
    code: "not_found",
  });
  await expect(root.forAccount(-1)).rejects.toMatchObject({ code: "usage" });
  await expect(root.forAccount(43)).resolves.toHaveProperty("accountId", 43);
});
it("fails closed without account senders/users rather than falling back", async () => {
  const m = server();
  const c = await new HeyClient({ token: "a", fetch: m.fetch }).forAccount(43);
  await expect(
    c.createMessage({
      body: { acting_sender_id: 0, message: { subject: "x", content: "x" } },
    }),
  ).rejects.toMatchObject({ code: "not_found" });
  await expect(
    c.createContact({
      body: { contact: { name: "x", email_address: "x@y.test" } },
    }),
  ).rejects.toMatchObject({ code: "not_found" });
  expect(m.requests).toHaveLength(1);
});
it("leaves explicit acting senders untouched; keeps identities and cache partitions separate", async () => {
  const m = server();
  const root = new HeyClient({ token: "a", fetch: m.fetch });
  const c = await root.forAccount(42);
  await c.createReply({
    path: { entryId: 1 },
    body: { acting_sender_id: 9, message: { subject: "x", content: "x" } },
  });
  expect(JSON.parse(await m.requests.at(-1)!.text()).acting_sender_id).toBe(9);
  await new HeyClient({ token: "b", fetch: m.fetch }).listBoxes();
  expect(m.requests.at(-1)!.headers.get("Authorization")).toBe("Bearer b");
});
it("uploads exact bytes/headers to signed URLs without HEY credentials, scope or redirects", async () => {
  const m = server();
  const c = await new HeyClient({ token: "a", fetch: m.fetch }).forAccount(42);
  await c.uploadBytes(
    {
      direct_upload: {
        url: "https://storage.example/upload?signature=secret",
        headers: {
          Authorization: "wrong",
          Cookie: "wrong",
          "Content-MD5": "sum",
        },
      },
      signed_id: "s",
      attachable_sgid: "a",
    },
    new Uint8Array([0, 1, 255]),
  );
  const request = m.requests.at(-1)!;
  expect(request.url).toBe("https://storage.example/upload?signature=secret");
  expect(request.headers.has("Authorization")).toBe(false);
  expect(request.headers.has("Cookie")).toBe(false);
  expect(request.headers.get("Content-MD5")).toBe("sum");
  expect(request.redirect).toBe("manual");
  expect(new Uint8Array(await request.arrayBuffer())).toEqual(
    new Uint8Array([0, 1, 255]),
  );
});
it("does not start a direct upload for an already-aborted request", async () => {
  const controller = new AbortController();
  controller.abort();
  const fetch = vi.fn();
  await expect(
    new HeyClient({ token: "a", fetch }).uploadBytes(
      {
        direct_upload: { url: "https://storage.example/upload", headers: {} },
        signed_id: "s",
        attachable_sgid: "a",
      },
      new Uint8Array([1]),
      { signal: controller.signal },
    ),
  ).rejects.toMatchObject({ name: "AbortError" });
  expect(fetch).not.toHaveBeenCalled();
});
it("partitions ETag revalidation by current credentials and evicts no-store", async () => {
  let token = "a";
  const requests: Request[] = [];
  let index = 0;
  const responses = [
    Response.json([], { headers: { ETag: '"a"' } }),
    Response.json([], {
      headers: { ETag: '"b"', "Cache-Control": "no-store" },
    }),
    Response.json([]),
  ];
  const c = new HeyClient({
    token: { getToken: () => token },
    cache: true,
    fetch: async (input, init) => {
      requests.push(new Request(input, init));
      return responses[index++]!;
    },
  });
  await c.listBoxes();
  token = "b";
  await c.listBoxes();
  await c.listBoxes();
  expect(requests.every((r) => !r.headers.has("If-None-Match"))).toBe(true);
});
