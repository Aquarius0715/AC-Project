// Installation requests (FR-C20, DEC-107): the customer owner's request for new aircons, and the rows both apps show.
import type { InstallationRequest } from "@ac/web/lib/contracts.gen";
import { codePoints } from "@ac/web/lib/text";
import { showTime, type I18n, type T } from "@ac/web/lib/i18n";
import { spacePath, type ApiPropertyRow, type ApiSpaceRow } from "@ac/web/lib/assets";
import type { ApiCapability } from "@ac/web/lib/devices";
import type { Tone } from "@ac/web/components/ui";

export type { InstallationRequest } from "@ac/web/lib/contracts.gen";

/** The most aircons one request asks for. */
const MAX_REQUEST_UNITS = 10;
/** The decline reason's length (installationRequests.decline). */
const DECLINE_REASON = { min: 5, max: 500 } as const;

const STATUS: Record<InstallationRequest["status"], { label: string; tone: Tone }> = {
  submitted: { label: "Submitted", tone: "warn" },
  approved: { label: "Approved", tone: "ok" },
  declined: { label: "Declined", tone: "crit" },
  cancelled: { label: "Cancelled", tone: "muted" },
};

export interface InstallationRow {
  id: string;
  version: number;
  /** the request number: the first 8 characters of its ID, as a job's */
  number: string;
  place: string;
  quantity: number;
  model: string;
  withIot: boolean;
  status: InstallationRequest["status"];
  statusLabel: string;
  tone: Tone;
  firstTime: string;
  note: string | null;
  declineReason: string | null;
  jobIds: string[];
  customerOrgId: string;
  propertyId: string;
  modelId: string | null;
  slots: InstallationRequest["preferredSlots"];
  contactWindow: string | null;
}

/** The names a request refers to, by ID. */
export interface RequestNames { properties: Pick<ApiPropertyRow, "id" | "name">[]; spaces: ApiSpaceRow[]; models: Pick<ApiCapability, "id" | "manufacturer" | "model">[] }

/** The rows of the requests, newest first as the Core API answers them, with the names of the place and the model. */
export function requestRows(requests: InstallationRequest[], names: RequestNames, i: I18n): InstallationRow[] {
  return requests.map((r) => {
    const property = names.properties.find((p) => p.id === r.propertyId)?.name ?? "—";
    const space = r.spaceId ? spacePath(r.spaceId, names.spaces) : "";
    const model = names.models.find((m) => m.id === r.modelId);
    const status = STATUS[r.status];
    return {
      id: r.id, version: r.version, number: r.id.slice(0, 8), place: space ? `${property} › ${space}` : property, quantity: r.quantity,
      model: model ? `${model.manufacturer} ${model.model}` : i.t("Not decided"), withIot: r.withIot, status: r.status, statusLabel: i.t(status.label), tone: status.tone,
      firstTime: r.preferredSlots[0] ? showTime(r.preferredSlots[0].startAt, i.display) : "—", note: r.note, declineReason: r.declineReason, jobIds: r.jobIds,
      customerOrgId: r.customerOrgId, propertyId: r.propertyId, modelId: r.modelId, slots: r.preferredSlots, contactWindow: r.contactWindow,
    };
  });
}

/** The number of units as typed: a whole number of 1–10, else the message. */
export function quantityError(text: string, t: T): string | null {
  const n = Number(text);
  return text.trim() === "" || !Number.isSafeInteger(n) || n < 1 || n > MAX_REQUEST_UNITS ? t("A whole number of 1–{max}", { max: MAX_REQUEST_UNITS }) : null;
}

/** The decline reason's message, or null when it is 5–500 characters. */
export function declineError(reason: string, t: T): string | null {
  const n = codePoints(reason.trim());
  return n < DECLINE_REASON.min || n > DECLINE_REASON.max ? t("A reason of {min}–{max} characters", { min: DECLINE_REASON.min, max: DECLINE_REASON.max }) : null;
}
