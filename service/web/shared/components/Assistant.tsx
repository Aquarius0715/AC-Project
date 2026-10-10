"use client";

import Link from "next/link";
import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { Btn, SummaryList, cx } from "./ui";
import { useI18n } from "./I18n";
import { actionMessage } from "@ac/web/lib/actionMessage";
import { candidateGroups, changeCard, examples, groupSummary, helpText, MICROPHONE, progressOf, roomOf, stateText, temperatureText, type CandidateGroup, type PanelState, type Progress, type ResolvedIntent } from "@ac/web/lib/assistant";
import { translator, type Locale, type T } from "@ac/web/lib/i18n";
import { callOp, OpError } from "@ac/web/lib/ops";
import { waitForCommand } from "@ac/web/lib/unitCommands";
import { useStoredValue } from "@ac/web/lib/urlState";
import type { ApiUnitDetail } from "@ac/web/lib/units";
import { sendVoiceChange } from "@ac/web/screens/shell/assistant/actions";

/** The header assistant (FR-X02, Figma Client 09a–09g). In API mode (`live`) it reads voice.resolveIntent with the
 * fixed en/ms grammar (D09), confirms a change before anything is sent and follows the command until the device
 * answers; the Phase 1A demo keeps its simulated answers. */
export function AssistantPanel({ open, onClose, context, live }: { open: boolean; onClose: () => void; context: string; live?: boolean }) {
  if (!open) return null;
  return live ? <LiveAssistant onClose={onClose} context={context} /> : <DemoAssistant onClose={onClose} context={context} />;
}

type Change = Extract<ResolvedIntent, { kind: "change" }>;
type Msg =
  | { kind: "user"; text: string; via: "text" | "voice" }
  | { kind: "bot"; text: string; tone?: "crit" }
  | { kind: "candidates"; question: string; via: "text" | "voice"; groups: CandidateGroup[]; status: Record<string, string> }
  | { kind: "confirm"; intent: Change; unit: ApiUnitDetail | null }
  | { kind: "progress"; id: number; p: Progress; unitId: string; question: string };

function Frame({ title, state, children, footer, lang, setLang, onClose, context, t }: {
  title: string; state: string; children: ReactNode; footer: ReactNode; lang: Locale; setLang?: (l: Locale) => void; onClose: () => void; context: string; t: T;
}) {
  return (
    <div className="fixed inset-0 z-40 flex justify-end bg-ink/30" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <aside role="dialog" aria-label={title} className="flex h-full w-full flex-col bg-surface shadow-2xl sm:max-w-[420px]">
        <div className="flex items-center justify-between border-b border-line px-4 py-3">
          <h2 className="flex items-center gap-2 font-bold">🎙 {title} <span className="rounded-full bg-surface2 px-2 py-0.5 text-[11px] font-semibold text-muted">{state}</span></h2>
          <div className="flex items-center gap-2">
            <select aria-label={t("Language")} value={lang} onChange={(e) => setLang?.(e.target.value as Locale)} className="rounded-control border border-line px-2 py-1 text-xs"><option value="en">English</option><option value="ms">Bahasa Melayu</option></select>
            <button aria-label={t("Close")} onClick={onClose} className="rounded-lg px-2 py-1 text-muted hover:bg-surface2">✕</button>
          </div>
        </div>
        <p className="border-b border-line bg-surface2/60 px-4 py-2 text-[11px] text-muted">{t("Context: {context} · you can control units you have permission for", { context })}</p>
        {children}
        {footer}
      </aside>
    </div>
  );
}

function LiveAssistant({ onClose, context }: { onClose: () => void; context: string }) {
  const i = useI18n();
  const [lang, setLangState] = useState<Locale>(i.display.locale);
  const t = translator(lang); // the assistant's own language; times stay in the user's time zone
  const display = { ...i.display, locale: lang };
  const ex = examples(lang);
  const [msgs, setMsgs] = useState<Msg[]>([]);
  const [state, setState] = useState<PanelState>("idle");
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const seq = useRef(0);
  const end = useRef<HTMLDivElement>(null);
  const input = useRef<HTMLInputElement>(null);
  // the device's microphone consent (Preferences): off leaves text only; turning it off discards a voice request
  const mic = useStoredValue(MICROPHONE) !== "off";
  const [micBefore, setMicBefore] = useState(mic);
  if (mic !== micBefore) {
    setMicBefore(mic);
    if (!mic && state === "listening") {
      setState("idle");
      setMsgs((s) => [...s, { kind: "bot", text: t("Voice input stopped — the microphone was turned off. Nothing was sent.") }]);
    }
  }
  // a block body: scrollIntoView returns a Promise in current browsers, which React would call as the cleanup
  useEffect(() => { end.current?.scrollIntoView({ block: "end" }); }, [msgs, state]);
  const add = (...m: Msg[]) => setMsgs((s) => [...s, ...m]);
  const drop = (kind: Msg["kind"]) => setMsgs((s) => s.filter((m) => m.kind !== kind));
  const failure = (e: unknown) => (e instanceof OpError ? actionMessage({ code: e.error.code, messageKey: e.error.messageKey, fieldErrors: e.error.fieldErrors ?? {} }, t) : t("The assistant is unavailable — try again."));
  const unit = (id: string) => callOp<ApiUnitDetail>("units.get", { id }).catch(() => null);
  // D09: a language change discards an unconfirmed intent but keeps what was typed
  const setLang = (l: Locale) => {
    setLangState(l);
    setMsgs((s) => s.filter((m) => m.kind !== "confirm" && m.kind !== "candidates"));
    if (state === "confirm" || state === "listening") setState("idle");
  };

  const ask = async (question: string, via: "text" | "voice", selectedUnitId?: string) => {
    if (!selectedUnitId) add({ kind: "user", text: question, via });
    setBusy(true);
    try {
      const r = await callOp<ResolvedIntent>("voice.resolveIntent", { text: question, locale: lang, ...(selectedUnitId ? { selectedUnitId } : {}) });
      if (r.kind === "help") {
        add({ kind: "bot", text: helpText(t) });
        setState("idle");
      } else if (r.kind === "unsupported") {
        add({ kind: "bot", text: t("Action unavailable. Try “{temperature}”, “{change}” or “{help}” — or type the AC's room.", ex) });
        setState("idle");
      } else if (r.kind === "candidates") {
        // never a guess: the room first, then the AC in it (Figma 09d); each AC with its connection and confirmed setting
        const units = await Promise.all(r.candidates.map((c) => unit(c.unitId)));
        const status = Object.fromEntries(r.candidates.map((c, n) => {
          const u = units[n];
          return [c.unitId, u ? t(u.connection === "online" ? "Online · confirmed {value}" : "Offline · last confirmed {value}", { value: u.observedState.celsius === null ? "—" : `${u.observedState.celsius} °C` }) : ""];
        }));
        add({ kind: "candidates", question, via, groups: candidateGroups(r.candidates), status });
        setState("confirm");
      } else if (r.kind === "temperature") {
        const u = await unit(r.unitId);
        add({ kind: "bot", text: temperatureText(r.measurement, u?.displayName ?? r.unitId.slice(0, 8), t, display) });
        setState("idle");
      } else {
        add({ kind: "confirm", intent: r, unit: await unit(r.unitId) });
        setState("confirm");
      }
    } catch (e) {
      add({ kind: "bot", text: failure(e), tone: "crit" });
      setState("failure");
    } finally {
      setBusy(false);
    }
  };

  const confirm = async (m: Extract<Msg, { kind: "confirm" }>, question: string) => {
    drop("confirm");
    setState("sending");
    const card = changeCard(m.intent, m.unit, t);
    const r = await sendVoiceChange(m.intent.unitId, m.intent.celsius, m.intent.expectedVersion);
    if (!r.ok) {
      add({ kind: "bot", text: actionMessage(r, t), tone: "crit" });
      setState("failure");
      return;
    }
    const id = ++seq.current;
    const measured = m.unit?.latestMeasurements.find((x) => x.metric === "temperature" && x.quality === "valid")?.value ?? null;
    const show = (c: typeof r.value) => {
      const p = progressOf(c, card.unit, m.intent.celsius, m.intent.before.celsius, measured, t, display);
      setMsgs((s) => (s.some((x) => x.kind === "progress" && x.id === id) ? s.map((x) => (x.kind === "progress" && x.id === id ? { ...x, p } : x)) : [...s, { kind: "progress", id, p, unitId: m.intent.unitId, question }]));
    };
    const final = await waitForCommand(r.value, show).catch(() => null);
    setState(final?.status === "acknowledged" ? "success" : final && ["failed", "expired", "cancelled"].includes(final.status) ? "failure" : "sending");
  };
  const voice = () => {
    if (mic) return setState("listening");
    add({ kind: "bot", text: t("The microphone is off on this device, so voice input is unavailable. Type your request instead, or turn the microphone on in Preferences.") });
    input.current?.focus();
  };
  const lastQuestion = [...msgs].reverse().find((m): m is Extract<Msg, { kind: "user" }> => m.kind === "user")?.text ?? "";

  return (
    <Frame title={t("Assistant")} state={stateText(state, t)} lang={lang} setLang={setLang} onClose={onClose} context={context} t={t}
      footer={<>
        <form className="flex items-center gap-2 border-t border-line p-3" onSubmit={(e) => { e.preventDefault(); if (text.trim() && !busy) { ask(text.trim(), "text"); setText(""); } }}>
          <input ref={input} value={text} onChange={(e) => setText(e.target.value)} placeholder={t("Type a message…")} aria-label={t("Message")} maxLength={200} className="min-w-0 flex-1 rounded-control border border-line px-3 py-2 text-[13px]" />
          <button type="button" disabled={busy || state === "listening"} onClick={voice} aria-label={t("Voice input")} className={cx("grid h-10 w-10 place-items-center rounded-full text-white disabled:opacity-50", mic ? "bg-primary" : "bg-muted")}>🎙</button>
        </form>
        <p className="px-4 pb-3 text-[11px] text-muted">{mic ? t("Voice or text · changes are always confirmed before sending · no business write before you confirm") : t("Text only — the microphone is off on this device · changes are always confirmed before sending")}</p>
      </>}>
      <div className="flex flex-1 flex-col gap-3 overflow-y-auto p-4" aria-live="polite">
        <div className="max-w-[90%] rounded-2xl rounded-bl-sm bg-surface2 px-3 py-2 text-[13px]">{t("Hi! Ask about temperatures, change a setting, or ask for help.")}
          <div className="mt-2 text-[11px] font-semibold text-muted">{t("Try")}</div>
          <div className="mt-1 flex flex-col gap-1 text-xs">{[ex.temperature, ex.change, ex.help].map((s) => <button key={s} disabled={busy} className="rounded-lg border border-line bg-surface px-2 py-1 text-left hover:bg-primary-soft disabled:opacity-50" onClick={() => ask(s, "text")}>“{s}”</button>)}</div>
        </div>
        {msgs.map((m, n) => {
          if (m.kind === "user") return <div key={n} className="ml-auto max-w-[85%] rounded-2xl rounded-br-sm bg-primary px-3 py-2 text-[13px] text-white">{m.text}<div className="text-[10px] opacity-70">{m.via === "voice" ? t("You · voice") : t("You · text")}</div></div>;
          if (m.kind === "bot") return <div key={n} className={cx("max-w-[90%] rounded-2xl rounded-bl-sm px-3 py-2 text-[13px]", m.tone === "crit" ? "bg-crit-soft text-crit" : "bg-surface2")}>{m.text}</div>;
          if (m.kind === "candidates") return (
            <CandidatePicker key={n} m={m} t={t} busy={busy}
              onPick={(unitId) => { drop("candidates"); ask(m.question, m.via, unitId); }}
              onCancel={() => { drop("candidates"); setState("idle"); add({ kind: "bot", text: t("Cancelled — nothing was sent.") }); }} />);
          if (m.kind === "confirm") {
            const card = changeCard(m.intent, m.unit, t);
            return (
              <div key={n} className="rounded-2xl border border-line bg-surface p-3 text-[13px]">
                <div className="mb-2 font-bold">{t("I understood:")}</div>
                <SummaryList items={[[t("Intent"), t("Change set temperature")], [t("Target"), card.target], [t("Current"), card.current], [t("Requested"), card.requested], [t("Allowed range"), card.range]]} />
                <p className="my-2 text-xs text-muted">{card.supported ? t("Only {unit} changes. Same permission checks as Unit Control.", { unit: card.unit }) : t("{value} is outside what {unit} supports — sending it will be refused, nothing changes.", { value: card.requested, unit: card.unit })}</p>
                <div className="flex gap-2"><Btn size="sm" onClick={() => { drop("confirm"); setState("idle"); add({ kind: "bot", text: t("Cancelled — nothing was sent.") }); }}>{t("Cancel")}</Btn><Btn variant="primary" size="sm" disabled={busy} onClick={() => confirm(m, lastQuestion)}>{t("Confirm")}</Btn></div>
              </div>);
          }
          return (
            <div key={n} className={cx("rounded-2xl px-3 py-2 text-[13px]", m.p.tone === "ok" ? "bg-ok-soft" : m.p.tone === "crit" ? "bg-crit-soft" : "bg-primary-soft")}>
              <b className={m.p.tone === "ok" ? "text-ok" : m.p.tone === "crit" ? "text-crit" : "text-primary"}>{m.p.tone === "ok" ? "✓ " : m.p.tone === "crit" ? "✕ " : "⟳ "}{m.p.title}</b>
              {m.p.rows && <div className="mt-1"><SummaryList items={m.p.rows} /></div>}
              <p className="mt-1 text-xs">{m.p.detail}</p>
              {m.p.tone !== "info" && <div className="mt-2 flex flex-wrap items-center gap-2">{m.p.tone === "crit" && <Btn size="sm" disabled={busy} onClick={() => ask(m.question, "text")}>{t("Try again")}</Btn>}<Link className="text-xs font-semibold text-primary" href={`/customer/units/${m.unitId}`}>{t("Open Unit Control")}</Link>{m.p.tone === "ok" && <span className="text-[11px] text-muted">{t("Undo is a new command")}</span>}</div>}
            </div>);
        })}
        {state === "listening" && (
          <div className="rounded-2xl bg-primary-soft p-3 text-[13px]">
            <b className="text-primary">● {t("Listening…")}</b>
            <p className="mt-1">“{ex.change}”</p>
            <p className="text-[11px] text-muted">{t("Live transcript (demo — no real microphone) · tap Stop when done")}</p>
            <div className="mt-2"><Btn size="sm" variant="primary" onClick={() => { setState("idle"); ask(ex.change, "voice"); }}>{t("Stop")}</Btn></div>
          </div>
        )}
        {busy && <p className="text-xs text-muted">{t("Thinking…")}</p>}
        <div ref={end} />
      </div>
    </Frame>
  );
}

/** Which room, then which AC (Figma 09d): nothing is chosen for the user, and Continue asks again with the chosen
 * unit (selectedUnitId, IR65). A room with one AC needs no second choice. */
function CandidatePicker({ m, t, busy, onPick, onCancel }: {
  m: Extract<Msg, { kind: "candidates" }>; t: T; busy: boolean; onPick: (unitId: string) => void; onCancel: () => void;
}) {
  const several = m.groups.length > 1;
  const [room, setRoom] = useState(several ? -1 : 0);
  const [picked, setPicked] = useState<string | null>(null);
  const g = m.groups[room];
  const chosen = g?.units.length === 1 ? g.units[0].unitId : picked;
  const id = useId();
  return (
    <div className="rounded-2xl bg-surface2 p-3 text-[13px]">
      <b>{several ? t("Which {room} did you mean?", { room: roomOf(m.groups) }) : t("Which AC in {path} did you mean?", { path: g.path })}</b>
      <p className="text-xs text-muted">{t("I found more than one. I won’t choose for you.")}</p>
      {several && (
        <fieldset className="mt-2 flex flex-col gap-1.5">
          <legend className="sr-only">{t("Room")}</legend>
          {m.groups.map((x, k) => <RadioCard key={x.path} name={`${id}-room`} checked={room === k} onChange={() => { setRoom(k); setPicked(null); }} title={x.path} detail={groupSummary(x, t)} />)}
        </fieldset>
      )}
      {g && g.units.length > 1 && (
        <fieldset className="mt-3 flex flex-col gap-1.5">
          <legend className={several ? "mb-1.5 font-semibold" : "sr-only"}>{several ? t("Then: which AC in {path}?", { path: g.path }) : t("AC")}</legend>
          {g.units.map((u) => <RadioCard key={u.unitId} name={`${id}-unit`} checked={picked === u.unitId} onChange={() => setPicked(u.unitId)} title={u.label} detail={m.status[u.unitId] ?? ""} />)}
        </fieldset>
      )}
      <div className="mt-3 flex justify-end gap-2">
        <Btn size="sm" onClick={onCancel}>{t("Cancel")}</Btn>
        <Btn variant="primary" size="sm" disabled={busy || !chosen} onClick={() => chosen && onPick(chosen)}>{t("Continue")}</Btn>
      </div>
    </div>
  );
}

function RadioCard({ name, checked, onChange, title, detail }: { name: string; checked: boolean; onChange: () => void; title: string; detail: string }) {
  return (
    <label className={cx("flex cursor-pointer items-start gap-2 rounded-lg border bg-surface px-2.5 py-2", checked ? "border-primary ring-1 ring-primary" : "border-line hover:bg-primary-soft")}>
      <input type="radio" name={name} checked={checked} onChange={onChange} className="mt-1 accent-[var(--color-primary)]" />
      <span><b className="block text-[13px]">{title}</b>{detail && <span className="block text-[11px] text-muted">{detail}</span>}</span>
    </label>
  );
}

// ---- the Phase 1A demo (DATA_SOURCE=demo): simulated answers, no Core API ----

type DemoMsg =
  | { kind: "user"; text: string; via: "text" | "voice" }
  | { kind: "bot"; text: string }
  | { kind: "confirm"; target: string; temp: number }
  | { kind: "ask"; options: string[]; temp: number }
  | { kind: "status"; tone: "info" | "ok" | "crit"; text: string };

const demoUnits = ["Bedroom AC", "Bedroom AC #2", "Living room AC", "Kitchen AC", "Study AC"];

function DemoAssistant({ onClose, context }: { onClose: () => void; context: string }) {
  const [msgs, setMsgs] = useState<DemoMsg[]>([{ kind: "bot", text: "Hi! Ask about temperatures, change a setting, or ask for help." }]);
  const [text, setText] = useState("");
  const [listening, setListening] = useState(false);
  const [lang, setLang] = useState<Locale>("en");
  const end = useRef<HTMLDivElement>(null);
  useEffect(() => { end.current?.scrollIntoView({ block: "end" }); }, [msgs, listening]);

  const interpret = (q: string, via: "text" | "voice") => {
    const add = (...m: DemoMsg[]) => setMsgs((s) => [...s, ...m]);
    add({ kind: "user", text: q, via });
    const temp = q.match(/(\d{2})/)?.[1];
    const named = demoUnits.find((u) => q.toLowerCase().includes(u.toLowerCase().replace(" #2", "")));
    if (/set|change/i.test(q) && temp) {
      if (named) return add({ kind: "confirm", target: named, temp: +temp });
      return add({ kind: "ask", options: ["Bedroom AC", "Bedroom AC #2"], temp: +temp });
    }
    if (/temperature/i.test(q)) return add({ kind: "bot", text: "Bedroom AC is 28.0 °C (observed 09:12). Bedroom AC #2 is 27.0 °C." });
    if (/help|schedule/i.test(q)) return add({ kind: "bot", text: "Schedules run an action at a set time on one AC. Create one in Automations & schedules › Create automation." });
    add({ kind: "bot", text: "I can read temperatures, change a setting (always confirmed first) or explain features." });
  };

  const send = (target: string, temp: number) => {
    setMsgs((s) => [...s.filter((m) => m.kind !== "confirm"), { kind: "status", tone: "info", text: `Sending: ${target} → ${temp}°C …` }]);
    setTimeout(() => {
      const fail = temp < 16 || temp > 30;
      setMsgs((s) => [...s, fail ? { kind: "status", tone: "crit", text: `Failed — ${temp}°C is outside the allowed range 16–30°C. Nothing was changed.` } : { kind: "status", tone: "ok", text: `Done — ${target} set to ${temp}°C (acknowledged by device).` }]);
    }, 1200);
  };

  const voice = () => {
    setListening(true);
    setTimeout(() => { setListening(false); interpret("Set Bedroom AC to 24 degrees", "voice"); }, 1600);
  };
  const t = translator("en");
  return (
    <Frame title="Assistant" state={listening ? "Listening" : "Idle"} lang={lang} setLang={setLang} onClose={onClose} context={context} t={t}
      footer={<>
        <form className="flex items-center gap-2 border-t border-line p-3" onSubmit={(e) => { e.preventDefault(); if (text.trim()) { interpret(text.trim(), "text"); setText(""); } }}>
          <input value={text} onChange={(e) => setText(e.target.value)} placeholder="Type a message…" className="min-w-0 flex-1 rounded-control border border-line px-3 py-2 text-[13px]" />
          <button type="button" onClick={voice} aria-label="Voice input" className="grid h-10 w-10 place-items-center rounded-full bg-primary text-white">🎙</button>
        </form>
        <p className="px-4 pb-3 text-[11px] text-muted">Voice or text · changes are always confirmed before sending · no business write before you confirm</p>
      </>}>
      <div className="flex flex-1 flex-col gap-3 overflow-y-auto p-4">
        {msgs.map((m, n) => {
          if (m.kind === "user") return <div key={n} className="ml-auto max-w-[85%] rounded-2xl rounded-br-sm bg-primary px-3 py-2 text-[13px] text-white">{m.text}<div className="text-[10px] opacity-70">You · {m.via}</div></div>;
          if (m.kind === "bot") return <div key={n} className="max-w-[90%] rounded-2xl rounded-bl-sm bg-surface2 px-3 py-2 text-[13px]">{m.text}{n === 0 && (
            <div className="mt-2 flex flex-col gap-1 text-xs">{["What’s the temperature in Bedroom?", "Set Bedroom AC to 24 degrees", "Help: how do schedules work?"].map((s) => <button key={s} className="rounded-lg border border-line bg-surface px-2 py-1 text-left hover:bg-primary-soft" onClick={() => interpret(s, "text")}>“{s}”</button>)}</div>)}</div>;
          if (m.kind === "confirm") return (
            <div key={n} className="rounded-2xl border border-line bg-surface p-3 text-[13px]">
              <div className="mb-2 font-bold">I understood:</div>
              <SummaryList items={[["Intent", "Change set temperature"], ["Target", `Home A › 1F › Bedroom › ${m.target}`], ["Current", "26°C (confirmed by device)"], ["Requested", `${m.temp}°C`], ["Allowed range", "16–30°C · step 1"]]} />
              <p className="my-2 text-xs text-muted">Only {m.target} changes. Same permission checks as Unit Control.</p>
              <div className="flex gap-2"><Btn variant="primary" size="sm" onClick={() => send(m.target, m.temp)}>Confirm & send</Btn><Btn size="sm" onClick={() => setMsgs((s) => [...s.filter((x) => x.kind !== "confirm"), { kind: "bot", text: "Cancelled — nothing was sent." }])}>Cancel</Btn></div>
            </div>);
          if (m.kind === "ask") return (
            <div key={n} className="rounded-2xl bg-surface2 p-3 text-[13px]">
              Which AC did you mean? I never guess the target.
              <div className="mt-2 flex flex-wrap gap-2">{m.options.map((o) => <Btn key={o} size="sm" onClick={() => setMsgs((s) => [...s.filter((x) => x.kind !== "ask"), { kind: "confirm", target: o, temp: m.temp }])}>{o}</Btn>)}</div>
            </div>);
          return <div key={n} className={cx("rounded-xl px-3 py-2 text-[13px] font-semibold", m.tone === "ok" ? "bg-ok-soft text-ok" : m.tone === "crit" ? "bg-crit-soft text-crit" : "bg-primary-soft text-primary")}>{m.tone === "ok" ? "✓ " : m.tone === "crit" ? "✕ " : "↻ "}{m.text}</div>;
        })}
        {listening && <div className="self-center rounded-full bg-primary-soft px-4 py-2 text-xs font-bold text-primary">🎙 Listening… speak now</div>}
        <div ref={end} />
      </div>
    </Frame>
  );
}
