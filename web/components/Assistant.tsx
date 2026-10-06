"use client";

import { useEffect, useRef, useState } from "react";
import { Btn, SummaryList, cx } from "./ui";

type Msg =
  | { kind: "user"; text: string; via: "text" | "voice" }
  | { kind: "bot"; text: string }
  | { kind: "confirm"; target: string; temp: number }
  | { kind: "ask"; options: string[]; temp: number }
  | { kind: "status"; tone: "info" | "ok" | "crit"; text: string };

const units = ["Bedroom AC", "Bedroom AC #2", "Living room AC", "Kitchen AC", "Study AC"];

export function AssistantPanel({ open, onClose, context }: { open: boolean; onClose: () => void; context: string }) {
  const [msgs, setMsgs] = useState<Msg[]>([{ kind: "bot", text: "Hi! Ask about temperatures, change a setting, or ask for help." }]);
  const [text, setText] = useState("");
  const [listening, setListening] = useState(false);
  const [lang, setLang] = useState("English");
  const end = useRef<HTMLDivElement>(null);
  useEffect(() => end.current?.scrollIntoView({ block: "end" }), [msgs, listening]);

  const interpret = (t: string, via: "text" | "voice") => {
    const add = (...m: Msg[]) => setMsgs((s) => [...s, ...m]);
    add({ kind: "user", text: t, via });
    const temp = t.match(/(\d{2})/)?.[1];
    const named = units.find((u) => t.toLowerCase().includes(u.toLowerCase().replace(" #2", "")));
    if (/set|change/i.test(t) && temp) {
      if (named) return add({ kind: "confirm", target: named, temp: +temp });
      return add({ kind: "ask", options: ["Bedroom AC", "Bedroom AC #2"], temp: +temp });
    }
    if (/temperature/i.test(t)) return add({ kind: "bot", text: "Bedroom AC is 28.0 °C (observed 09:12). Bedroom AC #2 is 27.0 °C." });
    if (/help|schedule/i.test(t)) return add({ kind: "bot", text: "Schedules run an action at a set time on one AC. Create one in Automations & schedules › Create automation." });
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

  if (!open) return null;
  return (
    <div className="fixed inset-0 z-40 flex justify-end bg-ink/30" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <aside role="dialog" aria-label="Assistant" className="flex h-full w-full flex-col bg-surface shadow-2xl sm:max-w-[420px]">
        <div className="flex items-center justify-between border-b border-line px-4 py-3">
          <h2 className="font-bold">🎙 Assistant</h2>
          <div className="flex items-center gap-2">
            <select aria-label="Language" value={lang} onChange={(e) => setLang(e.target.value)} className="rounded-control border border-line px-2 py-1 text-xs"><option>English</option><option>Bahasa Melayu</option></select>
            <button aria-label="Close" onClick={onClose} className="rounded-lg px-2 py-1 text-muted hover:bg-surface2">✕</button>
          </div>
        </div>
        <p className="border-b border-line bg-surface2/60 px-4 py-2 text-[11px] text-muted">Context: {context} · Home A · you can control units you have permission for</p>
        <div className="flex flex-1 flex-col gap-3 overflow-y-auto p-4">
          {msgs.map((m, i) => {
            if (m.kind === "user") return <div key={i} className="ml-auto max-w-[85%] rounded-2xl rounded-br-sm bg-primary px-3 py-2 text-[13px] text-white">{m.text}<div className="text-[10px] opacity-70">You · {m.via}</div></div>;
            if (m.kind === "bot") return <div key={i} className="max-w-[90%] rounded-2xl rounded-bl-sm bg-surface2 px-3 py-2 text-[13px]">{m.text}{i === 0 && (
              <div className="mt-2 flex flex-col gap-1 text-xs">{["What’s the temperature in Bedroom?", "Set Bedroom AC to 24 degrees", "Help: how do schedules work?"].map((s) => <button key={s} className="rounded-lg border border-line bg-surface px-2 py-1 text-left hover:bg-primary-soft" onClick={() => interpret(s, "text")}>“{s}”</button>)}</div>)}</div>;
            if (m.kind === "confirm") return (
              <div key={i} className="rounded-2xl border border-line bg-surface p-3 text-[13px]">
                <div className="mb-2 font-bold">I understood:</div>
                <SummaryList items={[["Intent", "Change set temperature"], ["Target", `Home A › 1F › Bedroom › ${m.target}`], ["Current", "26°C (confirmed by device)"], ["Requested", `${m.temp}°C`], ["Allowed range", "16–30°C · step 1"]]} />
                <p className="my-2 text-xs text-muted">Only {m.target} changes. Same permission checks as Unit Control.</p>
                <div className="flex gap-2"><Btn variant="primary" size="sm" onClick={() => send(m.target, m.temp)}>Confirm & send</Btn><Btn size="sm" onClick={() => setMsgs((s) => [...s.filter((x) => x.kind !== "confirm"), { kind: "bot", text: "Cancelled — nothing was sent." }])}>Cancel</Btn></div>
              </div>);
            if (m.kind === "ask") return (
              <div key={i} className="rounded-2xl bg-surface2 p-3 text-[13px]">
                Which AC did you mean? I never guess the target.
                <div className="mt-2 flex flex-wrap gap-2">{m.options.map((o) => <Btn key={o} size="sm" onClick={() => setMsgs((s) => [...s.filter((x) => x.kind !== "ask"), { kind: "confirm", target: o, temp: m.temp }])}>{o}</Btn>)}</div>
              </div>);
            return <div key={i} className={cx("rounded-xl px-3 py-2 text-[13px] font-semibold", m.tone === "ok" ? "bg-ok-soft text-ok" : m.tone === "crit" ? "bg-crit-soft text-crit" : "bg-primary-soft text-primary")}>{m.tone === "ok" ? "✓ " : m.tone === "crit" ? "✕ " : "↻ "}{m.text}</div>;
          })}
          {listening && <div className="self-center rounded-full bg-primary-soft px-4 py-2 text-xs font-bold text-primary">🎙 Listening… speak now</div>}
          <div ref={end} />
        </div>
        <form className="flex items-center gap-2 border-t border-line p-3" onSubmit={(e) => { e.preventDefault(); if (text.trim()) { interpret(text.trim(), "text"); setText(""); } }}>
          <input value={text} onChange={(e) => setText(e.target.value)} placeholder="Type a message…" className="min-w-0 flex-1 rounded-control border border-line px-3 py-2 text-[13px]" />
          <button type="button" onClick={voice} aria-label="Voice input" className="grid h-10 w-10 place-items-center rounded-full bg-primary text-white">🎙</button>
        </form>
        <p className="px-4 pb-3 text-[11px] text-muted">Voice or text · changes are always confirmed before sending · no business write before you confirm</p>
      </aside>
    </div>
  );
}
