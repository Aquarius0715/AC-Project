"use client";
// The equipment check of an installation job (FR-T16, DEC-106): the technician confirms that the installed model is
// the registered one, gives its serial and attaches a photo of the name plate, the indoor unit and the outdoor unit.
// The IoT device is registered and bound from the device screens with this job (FR-T11).
import Link from "next/link";
import { Badge, Card, Check, cx, Field, Input } from "@ac/web/components/ui";
import { useT } from "@ac/web/components/I18n";
import { Icon } from "@ac/web/components/Icon";
import { photoTagLabel, photoTags, type Draft, type PhotoTag } from "@ac/web/lib/techJob";

interface Props {
  draft: Draft;
  edit: (fn: (d: Draft) => Draft) => void;
  isEditable: boolean;
  model: string | null;
  photos: { id: string; status: string; tag: PhotoTag | null }[];
  pending: boolean;
  addPhoto: (files: FileList | null, tag: PhotoTag) => void;
}

export function EquipmentCard({ draft, edit, isEditable, model, photos, pending, addPhoto }: Readonly<Props>) {
  const t = useT();
  const equipment = draft.installation;
  if (!equipment) return null;
  return (
    <Card title={t("Equipment check")} sub={t("The installed unit against the register")}>
      <div className="flex flex-col gap-3">
        <Check
          label={t("The installed model is {model}", { model: model ?? "—" })}
          checked={equipment.confirmed}
          disabled={!isEditable || !model}
          onChange={(confirmed) => { edit((d) => ({ ...d, installation: d.installation && { ...d.installation, confirmed } })); }}
        />
        <Field label={t("Serial number")}>
          <Input value={equipment.serial} maxLength={80} readOnly={!isEditable} aria-label={t("Serial number")}
            onChange={(e) => { edit((d) => ({ ...d, installation: d.installation && { ...d.installation, serial: e.target.value } })); }} />
        </Field>
        <ul className="flex flex-col gap-2">
          {photoTags.map((tag) => {
            const added = photos.some((p) => p.tag === tag && p.status === "ready");
            return (
              <li key={tag} className="flex flex-wrap items-center justify-between gap-2">
                <span className="text-[13px] font-semibold">{t(photoTagLabel[tag])}</span>
                <span className="flex items-center gap-2">
                  <Badge tone={added ? "ok" : "warn"}>{t(added ? "Added" : "Missing")}</Badge>
                  {isEditable && (
                    <label className={cx("cursor-pointer text-xs font-semibold text-primary has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-offset-2 has-[:focus-visible]:outline-primary", pending && "pointer-events-none opacity-60")}>
                      {t(added ? "Replace" : "Add photo")}
                      <Input type="file" aria-label={t("Photo of the {part}", { part: t(photoTagLabel[tag]).toLowerCase() })} accept="image/jpeg,image/png" className="sr-only h-px w-px"
                        onChange={(e) => { addPhoto(e.target.files, tag); e.target.value = ""; }} />
                    </label>
                  )}
                </span>
              </li>
            );
          })}
        </ul>
        <p className="text-[11px] text-muted-foreground">
          {t("The IoT device is optional: register it and bind it to the unit with this job.")}{" "}
          <Link className="font-semibold text-primary" href="/technician/devices">{t("Devices")} <Icon name="arrowRight" /></Link>
        </p>
      </div>
    </Card>
  );
}
