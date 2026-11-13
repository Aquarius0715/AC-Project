import { headers } from "next/headers";
import { connection } from "next/server";
import { Icon } from "@ac/web/components/Icon";
import { serviceGroups } from "@/lib/services";

/** The portal (IR791): every AC Project service on one page, with no sign-in. The addresses are runtime settings, shown on
the address this request came by (IR651). */
export default async function PortalPage() {
  await connection();
  const requestHeaders = await headers();
  const groups = serviceGroups(process.env, requestHeaders.get("host"));
  return (
    <main className="mx-auto flex min-h-dvh w-full max-w-[880px] flex-col gap-8 px-4 py-12">
      <header className="flex items-center gap-3">
        <span aria-hidden className="grid h-10 w-10 place-items-center rounded-xl bg-primary text-xl text-white"><Icon name="cooling" /></span>
        <div>
          <h1 className="text-xl font-bold">AC Project services</h1>
          <p className="text-[13px] text-muted-foreground">Every service address in one place. Each application has its own sign-in.</p>
        </div>
      </header>
      {groups.map((g) => (
        <section key={g.id} aria-labelledby={`g-${g.id}`}>
          <h2 id={`g-${g.id}`} className="mb-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">{g.title}</h2>
          <ul className="grid gap-3 sm:grid-cols-2">
            {g.links.map((l) => (
              <li key={l.id}>
                <a href={l.href} className="flex h-full flex-col gap-1 rounded-2xl border border-line bg-surface p-4 shadow-sm hover:border-primary focus-visible:outline-2 focus-visible:outline-primary">
                  <span className="flex items-center gap-1.5 text-sm font-semibold text-primary">{l.name} <Icon name="external" /></span>
                  <span className="text-xs text-muted-foreground">{l.note}</span>
                </a>
              </li>
            ))}
          </ul>
        </section>
      ))}
    </main>
  );
}
