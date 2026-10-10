import { Card, LinkBtn, Page, SummaryList } from "@ac/web/components/ui";
import type { historyCard } from "@ac/web/lib/techJob";

/** A job whose viewing window ended (IR49): completed — the assignment ended with it (IR234) — or reassigned / cancelled,
 * read as the snapshot frozen when it ended (IR124), so a notification link still opens something. The texts and
 * times come from the loader in the user's display language and time zone (IR282). */
export function JobHistory({ card }: { card: ReturnType<typeof historyCard> }) {
  return (
    <Page narrow>
      <Card title={card.title} sub={card.sub}>
        <SummaryList items={card.rows} />
        <p className="mt-3 text-xs text-muted">{card.note}</p>
        <div className="mt-3"><LinkBtn href="/technician" size="sm">{card.back}</LinkBtn></div>
      </Card>
    </Page>
  );
}
