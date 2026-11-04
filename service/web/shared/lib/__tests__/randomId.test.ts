import { afterEach, describe, expect, it, vi } from "vitest";
import { randomId } from "@ac/web/lib/randomId";

const UUID_V4 = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;

describe("randomId (a UUID v4 in any page, DEC-93)", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });
  it("makes v4 UUIDs from getRandomValues outside a secure context, as on the HTTP test environment", () => {
    vi.stubGlobal("isSecureContext", false);
    const ids = Array.from({ length: 200 }, () => randomId());
    for (const id of ids) expect(id).toMatch(UUID_V4);
    expect(new Set(ids).size).toBe(ids.length);
  });
  it("uses crypto.randomUUID in a secure context", () => {
    vi.stubGlobal("isSecureContext", true);
    const uuid = vi.spyOn(crypto, "randomUUID").mockReturnValue("00000000-0000-4000-8000-000000000000");
    expect(randomId()).toBe("00000000-0000-4000-8000-000000000000");
    expect(uuid).toHaveBeenCalledOnce();
    uuid.mockRestore();
  });
});
