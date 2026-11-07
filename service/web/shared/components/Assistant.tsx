"use client";
const CANDIDATES = "candidates";
const CANCELLED_NOTHING_WAS_SENT = "Cancelled — nothing was sent.";

import Link from "next/link";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { Btn, Input, Select, SummaryList, cx } from "./ui";
import { RadioGroup, RadioGroupItem } from "@ac/web/components/ui/radio-group";
import { useI18n } from "./I18n";
import { actionMessage } from "@ac/web/lib/actionMessage";
import { candidateGroups, changeCard, examples, groupSummary, helpText, MICROPHONE, progressOf, roomOf, stateText, temperatureText, type CandidateGroup, type PanelState, type Progress, type ResolvedIntent } from "@ac/web/lib/assistant";
import { translator, type Locale, type T } from "@ac/web/lib/i18n";
import { callOp, OpError } from "@ac/web/lib/ops";
import { waitForCommand } from "@ac/web/lib/unitCommands";
import { useStoredValue } from "@ac/web/lib/urlState";
import type { ApiUnitDetail } from "@ac/web/lib/units";
import { sendVoiceChange } from "@ac/web/screens/shell/assistant/actions";
import { Icon, type IconName } from "./Icon";
import { Sheet, SheetContent } from "@ac/web/components/ui/sheet";
import { Button } from "@ac/web/components/ui/button";
import { oneOf } from "@ac/web/lib/oneOf";
import { orNull } from "@ac/web/lib/promises";

/** The header assistant (FR-X02, Figma Client 09a–09g). In API mode (`live`) it reads voice.resolveIntent with the
fixed en/ms grammar (D09), confirms a change before anything is sent and follows the command until the device
answers; the Phase 1A demo keeps its simulated answers. */
export function AssistantPanel({ open, onClose, context, live }: { open: boolean; onClose: () => void; context: string; live?: boolean }) {
  if (!open) return null;
  return live ? <LiveAssistant onClose={onClose} context={context} /> : <DemoAssistant onClose={onClose} context={context} />;
}

type Change = Extract<ResolvedIntent, { kind: "change" }>;

/** A command card's or status line's look by tone: done, failed, or anything else as still in progress. */
interface Look { soft: string; text: string; icon: IconName }
const TONE_LOOK = new Map<string, Look>([["ok", { soft: "bg-ok-soft", text: "text-ok", icon: "success" }], ["crit", { soft: "bg-crit-soft", text: "text-crit", icon: "error" }]]);
const PENDING_LOOK: Look = { soft: "bg-primary-soft", text: "text-primary", icon: "refresh" };
const lookOf = (tone: string): Look => TONE_LOOK.get(tone) ?? PENDING_LOOK;

/** An AC's connection and its last confirmed setting, for choosing between candidates. */
function connectionText(u: ApiUnitDetail, t: T): string {
  const value = u.observedState.celsius === null ? "—" : `${String(u.observedState.celsius)} °C`;
  return t(u.connection === "online" ? "Online · confirmed {value}" : "Offline · last confirmed {value}", { value });
}
type ChatMessage =
  | { kind: "user"; text: string; via: "text" | "voice" }
  | { kind: "bot"; text: string; tone?: "crit" }
  | { kind: "candidates"; question: string; via: "text" | "voice"; groups: CandidateGroup[]; status: Record<string, string> }
  | { kind: "confirm"; intent: Change; unit: ApiUnitDetail | null }
  | { kind: "progress"; id: number; p: Progress; unitId: string; question: string };

function Frame({ title, state, children, footer, lang, setLang, onClose, context, t }: Readonly<{
  title: string; state: string; children: ReactNode; footer: ReactNode; lang: Locale; setLang?: (l: Locale) => void; onClose: () => void; context: string; t: T;
}>) {
  return (
    <Sheet open onOpenChange={(next) => { if (!next) onClose(); }}>
      <SheetContent side="right" showCloseButton={false} aria-label={title} className="gap-0 bg-surface p-0 data-[side=right]:w-full data-[side=right]:sm:max-w-[420px]">
        <div className="flex items-center justify-between border-b border-line px-4 py-3">
          <h2 className="flex items-center gap-2 font-bold"><Icon name="mic" /> {title} <span className="rounded-full bg-surface2 px-2 py-0.5 text-[11px] font-semibold text-muted-foreground">{state}</span></h2>
          <div className="flex items-center gap-2">
            <Select aria-label={t("Language")} value={lang} onChange={(e) => setLang?.(oneOf(["en", "ms"] as const, e.target.value, lang))} className="w-auto text-xs"><option value="en">English</option><option value="ms">Bahasa Melayu</option></Select>
            <Button variant="ghost" size="icon-sm" aria-label={t("Close")} onClick={onClose} className="text-muted-foreground"><Icon name="close" /></Button>
          </div>
        </div>
        <p className="border-b border-line bg-surface2/60 px-4 py-2 text-[11px] text-muted-foreground">{t("Context: {context} · you can control units you have permission for", { context })}</p>
        {children}
        {footer}
      </SheetContent>
    </Sheet>
  );
}

function LiveAssistant({ onClose, context }: Readonly<{ onClose: () => void; context: string }>) {
  const i = useI18n();
  const [lang, setLangState] = useState<Locale>(i.display.locale);
  const t = translator(lang); // the assistant's own language; times stay in the user's time zone
  const display = { ...i.display, locale: lang };
  const ex = examples(lang);
  const [msgs, setMsgs] = useState<ChatMessage[]>([]);
  const [state, setState] = useState<PanelState>("idle");
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const seq = useRef(0);
  const end = useRef<HTMLDivElement>(null);
  const input = useRef<HTMLInputElement>(null);
  // the device's microphone consent (Preferences): off leaves text only; turning it off discards a voice request
  const isMic = useStoredValue(MICROPHONE) !== "off";
  const [micBefore, setMicBefore] = useState(isMic);
  if (isMic !== micBefore) {
    setMicBefore(isMic);
    if (!isMic && state === "listening") {
      setState("idle");
      setMsgs((s) => [...s, { kind: "bot", text: t("Voice input stopped — the microphone was turned off. Nothing was sent.") }]);
    }
  }
  // a block body: scrollIntoView returns a Promise in current browsers, which React would call as the cleanup
  useEffect(() => { end.current?.scrollIntoView({ block: "end" }); }, [msgs, state]);
  const add = (...m: ChatMessage[]) => { setMsgs((s) => [...s, ...m]); };
  const drop = (kind: ChatMessage["kind"]) => { setMsgs((s) => s.filter((m) => m.kind !== kind)); };
  const failure = (e: unknown) => (e instanceof OpError ? actionMessage({ code: e.error.code, messageKey: e.error.messageKey, fieldErrors: e.error.fieldErrors, retryAfterSeconds: e.error.retryAfterSeconds }, t) : t("The assistant is unavailable — try again."));
  const unit = (id: string) => orNull(callOp<ApiUnitDetail>("units.get", { id }));
  // D09: a language change discards an unconfirmed intent but keeps what was typed
  const setLang = (l: Locale) => {
    setLangState(l);
    setMsgs((s) => s.filter((m) => m.kind !== "confirm" && m.kind !== CANDIDATES));
    if (state === "confirm" || state === "listening") setState("idle");
  };

  const ask = async (question: string, via: "text" | "voice", selectedUnitId?: string) => {
    if (!selectedUnitId) add({ kind: "user", text: question, via });
    setBusy(true);
    try {
      const r = await callOp<ResolvedIntent>("voice.resolveIntent", { text: question, locale: lang, ...(selectedUnitId && { selectedUnitId }) });
      if (r.kind === "help") {
        add({ kind: "bot", text: helpText(t) });
        setState("idle");
      } else if (r.kind === "unsupported") {
        add({ kind: "bot", text: t("Action unavailable. Try “{temperature}”, “{change}” or “{help}” — or type the AC's room.", ex) });
        setState("idle");
      } else if (r.kind === CANDIDATES) {
        // never a guess: the room first, then the AC in it (Figma 09d); each AC with its connection and confirmed setting
        const units = await Promise.all(r.candidates.map((c) => unit(c.unitId)));
        const status = Object.fromEntries(r.candidates.map((c, n) => {
          const u = units.at(n);
          return [c.unitId, u ? connectionText(u, t) : ""];
        }));
        add({ kind: CANDIDATES, groups: candidateGroups(r.candidates), question, via, status });
        setState("confirm");
      } else if (r.kind === "temperature") {
        const u = await unit(r.unitId);
        add({ kind: "bot", text: temperatureText(r.measurement, u?.displayName ?? r.unitId.slice(0, 8), t, display) });
        setState("idle");
      } else {
        add({ kind: "confirm", intent: r, unit: await unit(r.unitId) });
        setState("confirm");
      }
    } catch (error) {
      add({ kind: "bot", text: failure(error), tone: "crit" });
      setState("failure");
    } finally {
      setBusy(false);
    }
  };

  const confirm = async (m: Extract<ChatMessage, { kind: "confirm" }>, question: string) => {
    drop("confirm");
    setState("sending");
    const card = changeCard(m.intent, m.unit, t);
    const r = await sendVoiceChange(m.intent.unitId, m.intent.celsius, m.intent.expectedVersion);
    if (!r.ok) {
      add({ kind: "bot", text: actionMessage(r, t), tone: "crit" });
      setState("failure");
      return;
    }
    seq.current += 1;
    const id = seq.current;
    const measured = m.unit?.latestMeasurements.find((x) => x.metric === "temperature" && x.quality === "valid")?.value ?? null;
    const show = (c: typeof r.value) => {
      const p = progressOf(c, card.unit, m.intent.celsius, m.intent.before.celsius, measured, t, display);
      setMsgs((s) => (s.some((x) => x.kind === "progress" && x.id === id) ? s.map((x) => (x.kind === "progress" && x.id === id ? { ...x, p } : x)) : [...s, { kind: "progress", unitId: m.intent.unitId, id, p, question }]));
    };
    const final = await orNull(waitForCommand(r.value, show));
    const unsettled = ["failed", "expired", "cancelled"].includes(final?.status ?? "") ? "failure" : "sending";
    setState(final?.status === "acknowledged" ? "success" : unsettled);
  };
  const voice = () => {
    if (isMic) { setState("listening"); return; }
    add({ kind: "bot", text: t("The microphone is off on this device, so voice input is unavailable. Type your request instead, or turn the microphone on in Preferences.") });
    input.current?.focus();
  };
  const lastQuestion = msgs.findLast((m): m is Extract<ChatMessage, { kind: "user" }> => m.kind === "user")?.text ?? "";

  return (
    <Frame title={t("Assistant")} state={stateText(state, t)} lang={lang} setLang={setLang} onClose={onClose} context={context} t={t}
      footer={<>
        <form className="flex items-center gap-2 border-t border-line p-3" onSubmit={(e) => { e.preventDefault(); if (!text.trim() || busy) {
          return;
        }

        void ask(text.trim(), "text"); setText(""); }}>
          <Input ref={input} value={text} onChange={(e) => { setText(e.target.value); }} placeholder={t("Type a message…")} aria-label={t("Message")} maxLength={200} className="min-w-0 flex-1 text-[13px]" />
          <Button type="button" size="icon" disabled={busy || state === "listening"} onClick={voice} aria-label={t("Voice input")} className={cx("size-10 rounded-full text-white disabled:opacity-50", isMic ? "bg-primary" : "bg-muted-foreground")}><Icon name="mic" className="text-lg" /></Button>
        </form>
        <p className="px-4 pb-3 text-[11px] text-muted-foreground">{t(isMic ? "Voice or text · changes are always confirmed before sending · no business write before you confirm" : "Text only — the microphone is off on this device · changes are always confirmed before sending")}</p>
      </>}>
      <div className="flex flex-1 flex-col gap-3 overflow-y-auto p-4" aria-live="polite">
        <div className="max-w-[90%] rounded-2xl rounded-bl-sm bg-surface2 px-3 py-2 text-[13px]">{t("Hi! Ask about temperatures, change a setting, or ask for help.")}
          <div className="mt-2 text-[11px] font-semibold text-muted-foreground">{t("Try")}</div>
          <div className="mt-1 flex flex-col gap-1 text-xs">{[ex.temperature, ex.change, ex.help].map((s) => <Button key={s} variant="outline" size="sm" disabled={busy} className="h-auto justify-start bg-surface px-2 py-1 text-left font-normal whitespace-normal hover:bg-primary-soft" onClick={() => { void ask(s, "text"); }}>“{s}”</Button>)}</div>
        </div>
        {msgs.map((m, n) => {
          if (m.kind === "user") return <div key={n} className="ml-auto max-w-[85%] rounded-2xl rounded-br-sm bg-primary px-3 py-2 text-[13px] text-white">{m.text}<div className="text-[10px]">{t(m.via === "voice" ? "You · voice" : "You · text")}</div></div>;
          if (m.kind === "bot") return <div key={n} className={cx("max-w-[90%] rounded-2xl rounded-bl-sm px-3 py-2 text-[13px]", m.tone === "crit" ? "bg-crit-soft text-crit" : "bg-surface2")}>{m.text}</div>;
          if (m.kind === CANDIDATES) return (
            <CandidatePicker key={n} m={m} t={t} busy={busy}
              onPick={(unitId) => { drop(CANDIDATES); void ask(m.question, m.via, unitId); }}
              onCancel={() => { drop(CANDIDATES); setState("idle"); add({ kind: "bot", text: t(CANCELLED_NOTHING_WAS_SENT) }); }} />);
          if (m.kind === "confirm") {
            const card = changeCard(m.intent, m.unit, t);
            return (
              <div key={n} className="rounded-2xl border border-line bg-surface p-3 text-[13px]">
                <div className="mb-2 font-bold">{t("I understood:")}</div>
                <SummaryList items={[[t("Intent"), t("Change set temperature")], [t("Target"), card.target], [t("Current"), card.current], [t("Requested"), card.requested], [t("Allowed range"), card.range]]} />
                <p className="my-2 text-xs text-muted-foreground">{card.supported ? t("Only {unit} changes. Same permission checks as Unit Control.", { unit: card.unit }) : t("{value} is outside what {unit} supports — sending it will be refused, nothing changes.", { value: card.requested, unit: card.unit })}</p>
                <div className="flex gap-2"><Btn size="sm" onClick={() => { drop("confirm"); setState("idle"); add({ kind: "bot", text: t(CANCELLED_NOTHING_WAS_SENT) }); }}>{t("Cancel")}</Btn><Btn variant="primary" size="sm" disabled={busy} onClick={() => { void confirm(m, lastQuestion); }}>{t("Confirm")}</Btn></div>
              </div>);
          }
          return (
            <div key={n} className={cx("rounded-2xl px-3 py-2 text-[13px]", lookOf(m.p.tone).soft)}>
              <b className={lookOf(m.p.tone).text}><Icon name={lookOf(m.p.tone).icon} /> {m.p.title}</b>
              {m.p.rows && <div className="mt-1"><SummaryList items={m.p.rows} /></div>}
              <p className="mt-1 text-xs">{m.p.detail}</p>
              {m.p.tone !== "info" && <div className="mt-2 flex flex-wrap items-center gap-2">{m.p.tone === "crit" && <Btn size="sm" disabled={busy} onClick={() => { void ask(m.question, "text"); }}>{t("Try again")}</Btn>}<Link className="text-xs font-semibold text-primary" href={`/customer/units/${m.unitId}`}>{t("Open Unit Control")}</Link>{m.p.tone === "ok" && <span className="text-[11px] text-muted-foreground">{t("Undo is a new command")}</span>}</div>}
            </div>);
        })}
        {state === "listening" && (
          <div className="rounded-2xl bg-primary-soft p-3 text-[13px]">
            <b className="text-primary"><Icon name="live" /> {t("Listening…")}</b>
            <p className="mt-1">“{ex.change}”</p>
            <p className="text-[11px] text-muted-foreground">{t("Live transcript (demo — no real microphone) · tap Stop when done")}</p>
            <div className="mt-2"><Btn size="sm" variant="primary" onClick={() => { setState("idle"); void ask(ex.change, "voice"); }}>{t("Stop")}</Btn></div>
          </div>
        )}
        {busy && <p className="text-xs text-muted-foreground">{t("Thinking…")}</p>}
        <div ref={end} />
      </div>
    </Frame>
  );
}

/** Which room, then which AC (Figma 09d): nothing is chosen for the user, and Continue asks again with the chosen
unit (selectedUnitId, IR65). A room with one AC needs no second choice. */
function CandidatePicker({ m, t, busy, onPick, onCancel }: Readonly<{
  m: Extract<ChatMessage, { kind: "candidates" }>; t: T; busy: boolean; onPick: (unitId: string) => void; onCancel: () => void;
}>) {
  const isSeveral = m.groups.length > 1;
  const [room, setRoom] = useState(isSeveral ? -1 : 0);
  const [picked, setPicked] = useState<string | null>(null);
  const g = room >= 0 ? m.groups.at(room) : undefined;
  const only = g?.units.length === 1 ? g.units[0] : undefined; // a room with one AC needs no choice
  const chosen = only?.unitId ?? picked;
  return (
    <div className="rounded-2xl bg-surface2 p-3 text-[13px]">
      <b>{isSeveral ? t("Which {room} did you mean?", { room: roomOf(m.groups) }) : t("Which AC in {path} did you mean?", { path: g?.path ?? "" })}</b>
      <p className="text-xs text-muted-foreground">{t("I found more than one. I won’t choose for you.")}</p>
      {isSeveral && (
        <fieldset className="mt-2 flex flex-col gap-1.5">
          <legend className="sr-only">{t("Room")}</legend>
          <RadioGroup value={String(room)} onValueChange={(v) => { setRoom(Number(v)); setPicked(null); }} className="gap-1.5">
            {m.groups.map((x, k) => <RadioCard key={x.path} value={String(k)} title={x.path} detail={groupSummary(x, t)} />)}
          </RadioGroup>
        </fieldset>
      )}
      {g && g.units.length > 1 && (
        <fieldset className="mt-3 flex flex-col gap-1.5">
          <legend className={isSeveral ? "mb-1.5 font-semibold" : "sr-only"}>{isSeveral ? t("Then: which AC in {path}?", { path: g.path }) : t("AC")}</legend>
          <RadioGroup value={picked ?? ""} onValueChange={(v: unknown) => { if (typeof v === "string") { setPicked(v); } }} className="gap-1.5">
            {g.units.map((u) => <RadioCard key={u.unitId} value={u.unitId} title={u.label} detail={m.status[u.unitId] ?? ""} />)}
          </RadioGroup>
        </fieldset>
      )}
      <div className="mt-3 flex justify-end gap-2">
        <Btn size="sm" onClick={onCancel}>{t("Cancel")}</Btn>
        <Btn variant="primary" size="sm" disabled={busy || !chosen} onClick={() => { if (chosen) onPick(chosen); }}>{t("Continue")}</Btn>
      </div>
    </div>
  );
}

function RadioCard({ value, title, detail }: Readonly<{ value: string; title: string; detail: string }>) {
  return (
    <label className="flex cursor-pointer items-start gap-2 rounded-lg border border-line bg-surface px-2.5 py-2 hover:bg-primary-soft has-data-checked:border-primary has-data-checked:ring-1 has-data-checked:ring-primary">
      <RadioGroupItem value={value} className="mt-1" />
      <span><b className="block text-[13px]">{title}</b>{detail && <span className="block text-[11px] text-muted-foreground">{detail}</span>}</span>
    </label>
  );
}

const demoUnits = ["Bedroom AC", "Bedroom AC #2", "Living room AC", "Kitchen AC", "Study AC"];

function DemoAssistant({ onClose, context }: Readonly<{ onClose: () => void; context: string }>) {
  // ---- the Phase 1A demo (DATA_SOURCE=demo): simulated answers, no Core API ----

  type DemoChatMessage =
  | { kind: "user"; text: string; via: "text" | "voice" }
  | { kind: "bot"; text: string }
  | { kind: "confirm"; target: string; temp: number }
  | { kind: "ask"; options: string[]; temp: number }
  | { kind: "status"; tone: "info" | "ok" | "crit"; text: string };
  const [msgs, setMsgs] = useState<DemoChatMessage[]>([{ kind: "bot", text: "Hi! Ask about temperatures, change a setting, or ask for help." }]);
  const [text, setText] = useState("");
  const [listening, setListening] = useState(false);
  const [lang, setLang] = useState<Locale>("en");
  const end = useRef<HTMLDivElement>(null);
  useEffect(() => { end.current?.scrollIntoView({ block: "end" }); }, [msgs, listening]);

  // choosing one of the offered ACs replaces the open question with the confirmation
  const choose = (target: string, celsius: number) => { setMsgs((s) => [...s.filter((x) => x.kind !== "ask"), { target, kind: "confirm", temp: celsius }]); };
  const interpret = (q: string, via: "text" | "voice") => {
    const add = (...m: DemoChatMessage[]) => { setMsgs((s) => [...s, ...m]); };
    add({ kind: "user", text: q, via });
    const setpoint = (/(\d{2})/.exec(q))?.[1];
    const named = demoUnits.find((u) => q.toLowerCase().includes(u.toLowerCase().replace(" #2", "")));
    if (setpoint && /set|change/i.test(q)) {
      add(named ? { kind: "confirm", target: named, temp: Number(setpoint) } : { kind: "ask", options: ["Bedroom AC", "Bedroom AC #2"], temp: Number(setpoint) });
      return;
    }
    if (/temperature/i.test(q)) { add({ kind: "bot", text: "Bedroom AC is 28.0 °C (observed 09:12). Bedroom AC #2 is 27.0 °C." }); return; }
    if (/help|schedule/i.test(q)) { add({ kind: "bot", text: "Schedules run an action at a set time on one AC. Create one in Automations & schedules › Create automation." }); return; }
    add({ kind: "bot", text: "I can read temperatures, change a setting (always confirmed first) or explain features." });
  };

  const send = (target: string, setpoint: number) => {
    setMsgs((s) => [...s.filter((m) => m.kind !== "confirm"), { kind: "status", tone: "info", text: `Sending: ${target} → ${String(setpoint)}°C …` }]);
    setTimeout(() => {
      const isFail = setpoint < 16 || setpoint > 30;
      setMsgs((s) => [...s, isFail ? { kind: "status", tone: "crit", text: `Failed — ${String(setpoint)}°C is outside the allowed range 16–30°C. Nothing was changed.` } : { kind: "status", tone: "ok", text: `Done — ${target} set to ${String(setpoint)}°C (acknowledged by device).` }]);
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
        <form className="flex items-center gap-2 border-t border-line p-3" onSubmit={(e) => { e.preventDefault(); if (!text.trim()) {
          return;
        }

        interpret(text.trim(), "text"); setText(""); }}>
          <Input value={text} onChange={(e) => { setText(e.target.value); }} placeholder="Type a message…" className="min-w-0 flex-1 text-[13px]" />
          <Button type="button" size="icon" onClick={voice} aria-label="Voice input" className="size-10 rounded-full text-white"><Icon name="mic" className="text-lg" /></Button>
        </form>
        <p className="px-4 pb-3 text-[11px] text-muted-foreground">Voice or text · changes are always confirmed before sending · no business write before you confirm</p>
      </>}>
      <div className="flex flex-1 flex-col gap-3 overflow-y-auto p-4">
        {msgs.map((m, n) => {
          if (m.kind === "user") return <div key={n} className="ml-auto max-w-[85%] rounded-2xl rounded-br-sm bg-primary px-3 py-2 text-[13px] text-white">{m.text}<div className="text-[10px]">You · {m.via}</div></div>;
          if (m.kind === "bot") return <div key={n} className="max-w-[90%] rounded-2xl rounded-bl-sm bg-surface2 px-3 py-2 text-[13px]">{m.text}{n === 0 && (
            <div className="mt-2 flex flex-col gap-1 text-xs">{["What’s the temperature in Bedroom?", "Set Bedroom AC to 24 degrees", "Help: how do schedules work?"].map((s) => <Button key={s} variant="outline" size="sm" className="h-auto justify-start bg-surface px-2 py-1 text-left font-normal whitespace-normal hover:bg-primary-soft" onClick={() => { interpret(s, "text"); }}>“{s}”</Button>)}</div>)}</div>;
          if (m.kind === "confirm") return (
            <div key={n} className="rounded-2xl border border-line bg-surface p-3 text-[13px]">
              <div className="mb-2 font-bold">I understood:</div>
              <SummaryList items={[["Intent", "Change set temperature"], ["Target", `Home A › 1F › Bedroom › ${m.target}`], ["Current", "26°C (confirmed by device)"], ["Requested", `${String(m.temp)}°C`], ["Allowed range", "16–30°C · step 1"]]} />
              <p className="my-2 text-xs text-muted-foreground">Only {m.target} changes. Same permission checks as Unit Control.</p>
              <div className="flex gap-2"><Btn variant="primary" size="sm" onClick={() => { send(m.target, m.temp); }}>Confirm & send</Btn><Btn size="sm" onClick={() => { setMsgs((s) => [...s.filter((x) => x.kind !== "confirm"), { kind: "bot", text: CANCELLED_NOTHING_WAS_SENT }]); }}>Cancel</Btn></div>
            </div>);
          if (m.kind === "ask") return (
            <div key={n} className="rounded-2xl bg-surface2 p-3 text-[13px]">
              Which AC did you mean? I never guess the target.
              <div className="mt-2 flex flex-wrap gap-2">{m.options.map((o) => <Btn key={o} size="sm" onClick={() => { choose(o, m.temp); }}>{o}</Btn>)}</div>
            </div>);
          return <div key={n} className={cx("rounded-xl px-3 py-2 text-[13px] font-semibold", lookOf(m.tone).soft, lookOf(m.tone).text)}><Icon name={lookOf(m.tone).icon} /> {m.text}</div>;
        })}
        {listening && <div className="self-center rounded-full bg-primary-soft px-4 py-2 text-xs font-bold text-primary"><Icon name="mic" /> Listening… speak now</div>}
        <div ref={end} />
      </div>
    </Frame>
  );
}
