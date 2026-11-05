import { z } from "zod";

/** The types the Core API takes for a photo (BlobInput.mime) and for a document (DocumentInput.mime). */
export const imageMime = z.enum(["image/jpeg", "image/png"]);
export const documentMime = z.enum(["application/pdf", "image/jpeg", "image/png"]);
