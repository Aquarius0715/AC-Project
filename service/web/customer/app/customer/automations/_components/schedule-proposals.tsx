"use client";

import { useT } from "@ac/web/components/I18n";
import { Btn, Card } from "@ac/web/components/ui";
import { dayLong, type ApiAutomation } from "@ac/web/lib/clientAutomations";
import type { StartProposal } from "@ac/web/lib/contracts.gen";
import { useAction } from "@ac/web/lib/useAction";
import { approveStartProposal } from "../actions";

interface Props {
  proposals: StartProposal[];
  units: { id: string; name: string }[];
  rules: ApiAutomation[];
}

export function ScheduleProposals({ proposals, units, rules }: Readonly<Props>) {
  const t = useT();
  const [pending, run] = useAction();

  return (
    <section aria-labelledby="schedule-proposals-heading" className="flex flex-col gap-3">
      <h2 id="schedule-proposals-heading" className="text-base font-semibold">{t("Schedule suggestions")}</h2>
      {proposals.length === 0 && <p className="text-sm text-muted-foreground">{t("No schedule suggestions yet.")}</p>}
      {proposals.map((proposal) => {
        const unit = units.find((item) => item.id === proposal.unitId);
        const rule = rules.find((item) => item.id === proposal.automationId && item.version === proposal.version);
        if (!unit || !rule) return null;

        const day = t(dayLong[proposal.weekday - 1] ?? "");
        const approve = () => {
          run(() => approveStartProposal({
            id: proposal.automationId, proposalId: proposal.id, weekday: proposal.weekday, version: proposal.version,
          }), t("Schedule change approved"));
        };

        return (
          <Card key={proposal.id}>
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div>
                <h3 className="text-sm font-semibold">{unit.name} · {rule.name}</h3>
                <p className="mt-1 text-sm font-semibold tabular-nums">
                  {day} {proposal.from} → {proposal.to}
                </p>
                <p className="mt-1 text-sm">
                  {t("On {support} of the last {total} occurrences of {day}, the AC was confirmed to turn on manually within 15 minutes of {time}.", {
                    day, support: proposal.supportDates.length, total: proposal.occurrences, time: proposal.to,
                  })}
                </p>
                <p className="mt-1 text-xs text-muted-foreground">
                  {t("Confirmed on {dates} · ends at {end} ({zone})", {
                    dates: proposal.supportDates.join(", "), end: proposal.endLocal, zone: rule.timezone,
                  })}
                </p>
              </div>
              <Btn size="sm" disabled={pending} onClick={approve}>{t("Approve schedule change")}</Btn>
            </div>
          </Card>
        );
      })}
      <p className="text-xs text-muted-foreground">{t("Full automation · Coming Soon")}</p>
    </section>
  );
}
