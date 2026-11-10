import type { z } from "zod";
import { CoreError } from "@ac/web/lib/dal";

/** A form value (text, a file's type) that has the shape `schema` describes. Anything else is refused as a VALIDATION
naming the field, as the Core API would, so a Server Action stops before it sends a value the contract does not know. */
export function formValue<T>(schema: z.ZodType<T>, value: unknown, field: string): T {
  const parsed = schema.safeParse(value);
  if (parsed.success) return parsed.data;
  throw new CoreError({ code: "VALIDATION", messageKey: "error.validation", fieldErrors: { [field]: "error.invalid" }, correlationId: "", retryAfterSeconds: null }, { status: 422 });
}
