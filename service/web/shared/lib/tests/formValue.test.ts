import { describe, expect, it, vi } from "vitest";
import { z } from "zod";

import { CoreError } from "@ac/web/lib/dal";
import { formValue } from "@ac/web/lib/formValue";
import { imageMime } from "@ac/web/lib/uploads";

// the Data Access Layer is server code: the cookie jar and redirect come from Next
vi.mock("server-only", () => ({}));
vi.mock("next/headers", () => ({ cookies: () => Promise.resolve({ get: vi.fn() }) }));
vi.mock("next/navigation", () => ({ redirect: (to: string) => { throw new Error(`NEXT_REDIRECT ${to}`); } }));

describe("a form value that must have the contract's shape", () => {
  it("gives the value when it fits", () => {
    expect(formValue(imageMime, "image/png", "photos")).toBe("image/png");
    expect(formValue(z.enum(["a", "b"]), "b", "reasonCode")).toBe("b");
  });

  it("refuses anything else as a VALIDATION naming the field", () => {
    const refused = (() => {
      try {
        formValue(imageMime, "application/pdf", "photos");
      } catch (error: unknown) {
        return error;
      }
      return null;
    })();
    expect(refused).toBeInstanceOf(CoreError);
    expect(refused instanceof CoreError && [refused.status, refused.error.code, refused.error.fieldErrors]).toEqual([422, "VALIDATION", { photos: "error.invalid" }]);
  });
});
