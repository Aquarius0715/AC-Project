import { Card, DemoBadge, cx } from "@ac/web/components/ui";
import { useT } from "@ac/web/components/I18n";

const jump = "rounded-control bg-primary-soft px-3 py-2 text-[13px] font-semibold text-ink hover:bg-primary-soft/70 disabled:cursor-not-allowed disabled:opacity-50";

/** The Demo controls layout of Figma Client 10e (FR-X05), shared by the browser demo and API mode: the dashed notice,
 * Demo clock · Scenario · Reset in one row, and the Trigger failures tiles. */
export function DemoFrame({ clock, scenario, reset, triggers, above }: { clock: React.ReactNode; scenario: React.ReactNode; reset: React.ReactNode; triggers: React.ReactNode; above?: React.ReactNode }) {
  const t = useT();
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-dashed border-[#fb923c] bg-warn-soft/60 px-3.5 py-2.5 text-[13px] font-semibold text-warn">
        <span>{t("Visually separated from business screens. Nothing here reaches real devices, payments, notifications, or IoT connections (FR-X05).")}</span><DemoBadge />
      </div>
      {above}
      <div className="grid-fluid" style={{ ["--min" as string]: "300px" }}>
        <Card title={<span className="flex items-center gap-2"><span aria-hidden>◷</span>{t("Demo clock")}</span>}>{clock}</Card>
        <Card title={<span className="flex items-center gap-2"><span aria-hidden>◍</span>{t("Scenario")}</span>}>{scenario}</Card>
        <Card title={<span className="flex items-center gap-2 text-crit"><span aria-hidden>↻</span>{t("Reset")}</span>}>{reset}</Card>
      </div>
      <Card title={t("Trigger failures")}>{triggers}</Card>
    </div>
  );
}

export function ClockFace({ time, onMinute, onHour, disabled }: { time: string; onMinute: () => void; onHour: () => void; disabled?: boolean }) {
  const t = useT();
  return (
    <>
      <div className="font-mono text-xl font-bold tracking-tight">{time}</div>
      <p className="mt-1 text-[11px] text-muted">{t("Real-time speed · advancing jumps do not consume session lifetime")}</p>
      <div className="mt-3 grid grid-cols-2 gap-2">
        <button type="button" className={jump} disabled={disabled} onClick={onMinute}>{t("+1 min")}</button>
        <button type="button" className={jump} disabled={disabled} onClick={onHour}>{t("+1 hour")}</button>
      </div>
    </>
  );
}

/** One trigger tile: icon, name and what it does; a tile that cannot run here says why. */
export function TriggerTile({ icon, title, sub, onClick, disabled, tone }: { icon: string; title: string; sub: string; onClick?: () => void; disabled?: boolean; tone?: "ok" }) {
  return (
    <button type="button" onClick={onClick} disabled={disabled || !onClick} className={cx("flex min-w-0 flex-col items-start gap-0.5 rounded-xl border border-line bg-surface px-3 py-2.5 text-left hover:bg-surface2 disabled:cursor-not-allowed disabled:opacity-60", tone === "ok" && "border-[#86efac]")}>
      <span className="flex items-center gap-2 text-[13px] font-semibold"><span aria-hidden className={cx("text-crit", tone === "ok" && "text-ok")}>{icon}</span>{title}</span>
      <span className="text-[11px] text-muted">{sub}</span>
    </button>
  );
}
