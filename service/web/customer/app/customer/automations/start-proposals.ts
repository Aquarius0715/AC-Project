import { z } from "zod";
import type { StartProposal } from "@ac/web/lib/contracts.gen";

const localTime = z.string().regex(/^([01]\d|2[0-3]):[0-5]\d$/);
const proposals = z.array(z.object({
  id: z.uuid(),
  automationId: z.uuid(),
  unitId: z.uuid(),
  version: z.number().int().positive(),
  weekday: z.number().int().min(1).max(7),
  from: localTime,
  to: localTime,
  endLocal: localTime,
  occurrences: z.literal(4),
  supportDates: z.array(z.iso.date()).min(3).max(4),
}));

export function parseStartProposals(value: unknown): StartProposal[] {
  return proposals.parse(value);
}
