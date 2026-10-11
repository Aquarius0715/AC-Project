"use client";

import Link from "next/link";
import { Children, createContext, isValidElement, useCallback, useContext, useId, useRef, useState } from "react";
import { useT } from "@ac/web/components/I18n";
import { Icon, type IconName } from "@ac/web/components/Icon";
import { Alert } from "@ac/web/components/ui/alert";
import { Badge as UIBadge } from "@ac/web/components/ui/badge";
import { Button, buttonVariants } from "@ac/web/components/ui/button";
import { Card as UICard, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from "@ac/web/components/ui/card";
import { Checkbox } from "@ac/web/components/ui/checkbox";
import { Dialog, DialogClose, DialogContent, DialogTitle } from "@ac/web/components/ui/dialog";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@ac/web/components/ui/dropdown-menu";
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@ac/web/components/ui/empty";
import { Input as UIInput } from "@ac/web/components/ui/input";
import { NativeSelect } from "@ac/web/components/ui/native-select";
import { Switch } from "@ac/web/components/ui/switch";
import { Tabs as UITabs, TabsList, TabsTrigger } from "@ac/web/components/ui/tabs";
import { Textarea as UITextarea } from "@ac/web/components/ui/textarea";
import { ToggleGroup, ToggleGroupItem } from "@ac/web/components/ui/toggle-group";
import { cn } from "@ac/web/lib/utils";

// The product's components (DEC-75): the screens' names (Btn, Card, Badge, …) built on the shadcn/ui components of
// components/ui/. A later phase moves the screens to the shadcn components themselves.

/** The class helper of the screens: shadcn/ui's cn, so a later class wins over a conflicting earlier one (w-auto over w-full). */
export const cx = cn;

/* ───────────── Badges ───────────── */
export type Tone = "ok" | "warn" | "crit" | "unknown" | "primary" | "muted";
const toneCls: Record<Tone, string> = {
  ok: "bg-ok-soft text-ok",
  warn: "bg-warn-soft text-warn",
  crit: "bg-crit-soft text-crit",
  unknown: "bg-unknown-soft text-unknown",
  primary: "bg-primary-soft text-primary",
  muted: "bg-surface2 text-muted-foreground",
};

/** A status badge: the shadcn/ui Badge in the tone's colours. */
export function Badge({ tone = "muted", icon, children, className }: { tone?: Tone; icon?: IconName; children: React.ReactNode; className?: string }) {
  return (
    <UIBadge variant="secondary" className={cn(toneCls[tone], className)}>
      {icon && <Icon name={icon} />}
      {children}
    </UIBadge>
  );
}

// the state badges speak the display language (IR258); the state values themselves stay as stored
export function PowerBadge({ s }: { s: "running" | "stopped" | "unknown" }) {
  const t = useT();
  return s === "running" ? <Badge tone="ok" icon="running">{t("Running")}</Badge> : s === "stopped" ? <Badge tone="muted" icon="stopped">{t("Stopped")}</Badge> : <Badge tone="unknown" icon="unknown">{t("Unknown")}</Badge>;
}
export function ConnBadge({ s }: { s: "online" | "offline" | "connecting" | "unknown" | "error" }) {
  const t = useT();
  return s === "online" ? <Badge tone="ok" icon="online">{t("Online")}</Badge> : s === "offline" ? <Badge tone="unknown" icon="offline">{t("Offline")}</Badge> : s === "error" ? <Badge tone="crit" icon="error">{t("Error")}</Badge>
    : s === "unknown" ? <Badge tone="unknown" icon="unknown">{t("Unknown")}</Badge> : <Badge tone="primary" icon="refresh">{t("Connecting")}</Badge>;
}
export function SeverityBadge({ s }: { s: "critical" | "warning" | "normal" }) {
  const t = useT();
  return s === "critical" ? <Badge tone="crit" icon="critical">{t("Critical")}</Badge> : s === "warning" ? <Badge tone="warn" icon="warning">{t("Warning")}</Badge> : <Badge tone="primary" icon="info">{t("Info")}</Badge>;
}
export function OnOffBadge({ on }: { on: boolean }) {
  const t = useT();
  return on ? <Badge tone="ok">{t("On")}</Badge> : <Badge tone="muted">{t("Off")}</Badge>;
}
export const DemoBadge = () => <Badge tone="warn" className="uppercase tracking-wide">Demo</Badge>;

/* ───────────── Buttons ───────────── */
type BtnVariant = "primary" | "secondary" | "ghost" | "danger";
// the product's button names on the shadcn/ui Button variants; ghost keeps the primary colour of a text action
const variantOf = { primary: "default", secondary: "outline", ghost: "ghost", danger: "destructive" } as const;
const variantCls = (v: BtnVariant) => cn(v === "secondary" && "bg-card", v === "ghost" && "text-primary hover:bg-accent hover:text-primary");
const sizeOf = (size: "sm" | "md") => (size === "sm" ? "sm" : "default");
const btnCls = (v: BtnVariant, size: "sm" | "md") => cn(buttonVariants({ variant: variantOf[v], size: sizeOf(size) }), variantCls(v));

export function Btn({ variant = "secondary", size = "md", className, ...p }: React.ButtonHTMLAttributes<HTMLButtonElement> & { variant?: BtnVariant; size?: "sm" | "md" }) {
  return <Button type="button" {...p} variant={variantOf[variant]} size={sizeOf(size)} className={cn(variantCls(variant), className)} />;
}
export function LinkBtn({ href, variant = "secondary", size = "md", className, children }: { href: string; variant?: BtnVariant; size?: "sm" | "md"; className?: string; children: React.ReactNode }) {
  return <Link href={href} className={cx(btnCls(variant, size), className)}>{children}</Link>;
}
export function TextLink({ href, children, className }: { href: string; children: React.ReactNode; className?: string }) {
  return <Link href={href} className={cx("text-xs font-semibold text-primary hover:underline", className)}>{children}</Link>;
}

/* ───────────── Layout primitives ───────────── */
/** The main column: 1280 px wide, or the 640 px single-column of offers and snapshots (Figma) with `narrow`. */
export function Page({ children, className, narrow }: { children: React.ReactNode; className?: string; narrow?: boolean }) {
  return <div className={cx("page mx-auto flex w-full flex-col gap-4", narrow ? "max-w-[640px]" : "max-w-[1280px]", className)}>{children}</div>;
}

/** A card: the shadcn/ui Card. The title stays a level-2 heading; `pad={false}` lets a table reach the edges; the
 * overflow stays visible for the menus that still open inside a card. */
export function Card({ title, sub, action, children, className, pad = true, tone }: { title?: React.ReactNode; sub?: React.ReactNode; action?: React.ReactNode; children?: React.ReactNode; className?: string; pad?: boolean; tone?: "crit" | "warn" | "ok" }) {
  return (
    <UICard className={cn("min-w-0 overflow-visible", !pad && "pb-0", tone === "crit" && "ring-crit", tone === "warn" && "ring-[#fdba74]", tone === "ok" && "ring-[#86efac]", className)}>
      {(title || action) && (
        <CardHeader>
          {title && <CardTitle><h2>{title}</h2></CardTitle>}
          {sub && <CardDescription className="text-xs">{sub}</CardDescription>}
          {action && <CardAction>{action}</CardAction>}
        </CardHeader>
      )}
      <CardContent className={cn("min-w-0", !pad && "px-0")}>{children}</CardContent>
    </UICard>
  );
}

export function PageHead({ title, sub, action }: { title: React.ReactNode; sub?: React.ReactNode; action?: React.ReactNode }) {
  return (
    <div className="flex flex-wrap items-end justify-between gap-3">
      <div className="min-w-0">
        <h1 className="text-lg font-bold text-ink">{title}</h1>
        {sub && <p className="text-xs text-muted-foreground">{sub}</p>}
      </div>
      {action}
    </div>
  );
}

export function Kpi({ label, value, icon, unit, badge, sub, href, link, tone }: { label: string; value: React.ReactNode; icon?: IconName; unit?: string; badge?: React.ReactNode; sub?: React.ReactNode; href?: string; link?: string; tone?: "crit" | "warn" | "ok" }) {
  return (
    <UICard className="min-w-0 gap-1 px-4">
      <div className="flex items-center justify-between gap-2 text-xs text-muted-foreground">
        <span>{label}</span>
        {badge}
      </div>
      <div className={cx("text-[28px] font-bold leading-9", tone === "warn" && "text-warn", tone === "crit" && "text-crit", tone === "ok" && "text-ok")}>
        {value}
        {icon && <Icon name={icon} filled className="ml-1 text-[20px] text-warn" />}
        {unit && <span className="ml-1 text-sm font-medium text-muted-foreground">{unit}</span>}
      </div>
      {sub && <p className="text-xs text-muted-foreground">{sub}</p>}
      {href && link && <TextLink href={href} className="mt-auto pt-1">{link} <Icon name="arrowRight" /></TextLink>}
    </UICard>
  );
}

export function Stat({ label, value, sub }: { label: string; value: React.ReactNode; sub?: React.ReactNode }) {
  return (
    <div className="min-w-0 rounded-xl bg-surface2 p-3">
      <div className="text-[11px] text-muted-foreground">{label}</div>
      <div className="text-base font-bold text-ink">{value}</div>
      {sub && <div className="text-[11px] text-muted-foreground">{sub}</div>}
    </div>
  );
}

/** A message line: the shadcn/ui Alert in the tone's colours, an alert only when critical (otherwise a status). */
export function Banner({ tone = "primary", icon, children, action }: { tone?: Tone; icon?: IconName; children: React.ReactNode; action?: React.ReactNode }) {
  const bd = tone === "warn" ? "border-[#fdba74] bg-warn-soft/60" : tone === "crit" ? "border-crit bg-crit-soft/60" : tone === "ok" ? "border-[#86efac] bg-ok-soft/60" : "border-border bg-primary-soft/50";
  return (
    <Alert role={tone === "crit" ? "alert" : "status"} className={cn("flex flex-wrap items-center gap-x-3 gap-y-2 rounded-xl px-3.5 py-2.5 text-[13px] *:[svg]:translate-y-0", bd)}>
      <Icon name={icon ?? (tone === "warn" ? "warning" : tone === "crit" ? "error" : tone === "ok" ? "success" : "info")} className="text-[15px]" />
      <div className="min-w-0 flex-1">{children}</div>
      {action}
    </Alert>
  );
}

/** No rows, a failed read, no connection: the shadcn/ui Empty. The title stays a level-3 heading. */
export function EmptyState({ title, children, action }: { title: string; children?: React.ReactNode; action?: React.ReactNode }) {
  return (
    <Empty className="border bg-card">
      <EmptyHeader>
        <EmptyMedia variant="icon"><Icon name="info" /></EmptyMedia>
        <EmptyTitle><h3>{title}</h3></EmptyTitle>
        {children && <EmptyDescription>{children}</EmptyDescription>}
      </EmptyHeader>
      {action && <EmptyContent>{action}</EmptyContent>}
    </Empty>
  );
}
export function ErrorState({ title, children, onRetry }: { title: string; children?: React.ReactNode; onRetry?: () => void }) {
  return (
    <Empty className="border border-solid border-crit bg-card">
      <EmptyHeader>
        <EmptyMedia variant="icon" className="bg-crit-soft text-crit"><Icon name="error" /></EmptyMedia>
        <EmptyTitle className="text-crit"><h3>{title}</h3></EmptyTitle>
        {children && <EmptyDescription>{children}</EmptyDescription>}
      </EmptyHeader>
      <EmptyContent><Btn size="sm" onClick={onRetry}><Icon name="refresh" /> Retry</Btn></EmptyContent>
    </Empty>
  );
}
export function OfflineState({ children }: { children: React.ReactNode }) {
  return (
    <Empty className="border bg-card">
      <EmptyHeader>
        <EmptyMedia variant="icon"><Icon name="offline" /></EmptyMedia>
        <EmptyTitle><h3>Offline — data may be stale</h3></EmptyTitle>
        <EmptyDescription>{children}</EmptyDescription>
      </EmptyHeader>
    </Empty>
  );
}

export function SummaryList({ items, cols = 1 }: { items: [string, React.ReactNode][]; cols?: 1 | 2 }) {
  return (
    <dl className={cx("grid gap-x-6 text-[13px]", cols === 2 ? "grid-cols-[repeat(auto-fit,minmax(min(100%,260px),1fr))]" : "grid-cols-1")}>
      {items.map(([k, v]) => (
        <div key={k} className="grid grid-cols-[minmax(90px,38%)_1fr] gap-2 border-b border-line/70 py-1.5 last:border-b-0">
          <dt className="text-muted-foreground">{k}</dt>
          <dd className="min-w-0 break-words font-medium">{v}</dd>
        </div>
      ))}
    </dl>
  );
}

export function Steps({ steps, current }: { steps: string[]; current: number }) {
  return (
    <ol className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs font-semibold">
      {steps.map((s, i) => (
        <li key={s} className={cx("flex items-center gap-1.5", i === current ? "text-primary" : i < current ? "text-ok" : "text-muted-foreground")}>
          <span className={cx("grid h-5 w-5 place-items-center rounded-full text-[11px]", i === current ? "bg-primary text-white" : i < current ? "bg-ok-soft" : "bg-surface2")}>{i < current ? <Icon name="check" /> : i + 1}</span>
          {s}
        </li>
      ))}
    </ol>
  );
}

export function Timeline({ items }: { items: { time: string; title: React.ReactNode; detail?: React.ReactNode; tone?: Tone }[] }) {
  return (
    <ol className="flex flex-col gap-3">
      {items.map((it, i) => (
        <li key={i} className="grid grid-cols-[84px_14px_1fr] items-start gap-2 text-[13px]">
          <span className="text-xs text-muted-foreground">{it.time}</span>
          <span className={cx("mt-1.5 h-2.5 w-2.5 rounded-full", it.tone === "crit" ? "bg-crit" : it.tone === "warn" ? "bg-warn" : it.tone === "ok" ? "bg-ok" : "bg-primary")} />
          <div className="min-w-0">
            <div className="font-semibold">{it.title}</div>
            {it.detail && <div className="text-xs text-muted-foreground">{it.detail}</div>}
          </div>
        </li>
      ))}
    </ol>
  );
}

/* ───────────── Tabs / segmented ───────────── */
/** A tab bar: the shadcn/ui Tabs (tablist and tabs; the screens draw the panels). `count` is a badge after the label. */
export function Tabs<T extends string>({ tabs, value, onChange, className }: { tabs: { id: T; label: string; count?: number | string }[]; value: T; onChange: (v: T) => void; className?: string }) {
  return (
    <UITabs value={value} onValueChange={(v) => onChange(v as T)} className={cn("max-w-full", className)}>
      <TabsList className="max-w-full flex-wrap justify-start group-data-horizontal/tabs:h-auto">
        {tabs.map((t) => (
          <TabsTrigger key={t.id} value={t.id} className="flex-none px-3 py-1.5 text-[13px] font-semibold text-muted-foreground">
            {t.label}
            {t.count !== undefined && <span className="rounded-full bg-primary-soft px-1.5 text-[11px] text-primary">{t.count}</span>}
          </TabsTrigger>
        ))}
      </TabsList>
    </UITabs>
  );
}

/* ───────────── Forms ───────────── */
export function Field({ label, hint, error, children, className }: { label: string; hint?: React.ReactNode; error?: React.ReactNode; children: React.ReactNode; className?: string }) {
  // Only a single form control sits inside a <label>. Anything else — choice buttons, a set of checkboxes or buttons, a
  // read-only box — is a labelled group: inside a <label> a button takes the label as its name (IR230, IR238).
  const control = isValidElement(children) && (children.type === Input || children.type === Select || children.type === Textarea || ["input", "select", "textarea"].includes(children.type as string));
  const group = !control;
  const Tag = group ? "div" : "label";
  return (
    <Tag role={group ? "group" : undefined} aria-label={group ? label : undefined} className={cx("flex min-w-0 flex-col gap-1 text-xs text-ink", className)}>
      <span aria-hidden={group || undefined} className="font-semibold">{label}</span>
      {children}
      {error ? <span className="flex items-center gap-1 font-medium text-crit"><Icon name="error" />{error}</span> : hint ? <span className="font-normal text-muted-foreground">{hint}</span> : null}
    </Tag>
  );
}
// The form controls are the shadcn/ui Input, Textarea and NativeSelect. A width in className (w-auto, w-40) replaces the
// default one (cn merges), and a select stays a native select, so the platform's own picker works everywhere.
export const Input = ({ className, ...p }: React.ComponentProps<"input">) => <UIInput {...p} className={className} />; // React 19: ref is a prop
export const Select = ({ className, children, ...p }: Omit<React.SelectHTMLAttributes<HTMLSelectElement>, "size">) => (
  <NativeSelect {...p} className={cn("w-full", className)}>{children}</NativeSelect>
);
/** A row's actions behind the "more" icon: the shadcn/ui DropdownMenu. The actions are written as <option value>
 * children (shown only when they apply); `onChoose` gets the chosen value. */
export function ActionMenu({ label, disabled, onChoose, children }: { label: string; disabled?: boolean; onChoose: (value: string) => void; children: React.ReactNode }) {
  const items = Children.toArray(children).filter((c): c is React.ReactElement<{ value: string; children: React.ReactNode }> => isValidElement(c) && c.type === "option");
  return (
    <DropdownMenu>
      <DropdownMenuTrigger disabled={disabled} render={<Button variant="ghost" size="icon-sm" aria-label={label} className="text-muted-foreground" />}>
        <Icon name="more" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-44">
        {items.map((o) => <DropdownMenuItem key={o.props.value} onClick={() => onChoose(o.props.value)}>{o.props.children}</DropdownMenuItem>)}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
export const Textarea = ({ className, ...p }: React.TextareaHTMLAttributes<HTMLTextAreaElement>) => <UITextarea {...p} className={cn("min-h-[84px] resize-y", className)} />;
/** A one-time code as separate boxes (Figma 10g): one transparent input over the boxes, so typing, pasting and the
 * browser's one-time-code autofill all work; the box for the next digit is outlined while the input has focus. */
export function CodeInput({ value, onChange, label, length = 6, autoFocus }: { value: string; onChange: (v: string) => void; label: string; length?: number; autoFocus?: boolean }) {
  const [focused, setFocused] = useState(false);
  const next = Math.min(value.length, length - 1);
  return (
    <div className="relative inline-flex gap-2">
      {Array.from({ length }, (_, i) => (
        <span key={i} aria-hidden className={cx("grid h-10 w-10 place-items-center rounded-lg border bg-surface text-lg font-semibold", focused && i === next ? "border-primary ring-2 ring-primary/20" : "border-line")}>{value[i] ?? ""}</span>
      ))}
      <input aria-label={label} inputMode="numeric" autoComplete="one-time-code" maxLength={length} value={value} autoFocus={autoFocus}
        onChange={(e) => onChange(e.target.value.replace(/\D/g, "").slice(0, length))} onFocus={() => setFocused(true)} onBlur={() => setFocused(false)}
        className="absolute inset-0 h-full w-full cursor-text opacity-0" />
    </div>
  );
}
/** A QR code drawn from its module path (lib/twoFactor qrPath), dark on white whatever the theme so it scans. */
export function QrCode({ qr, label, className }: { qr: { size: number; path: string }; label: string; className?: string }) {
  return (
    <svg role="img" aria-label={label} viewBox={`0 0 ${qr.size} ${qr.size}`} shapeRendering="crispEdges" className={cx("block bg-white", className)}>
      <path d={qr.path} fill="#0b1220" />
    </svg>
  );
}
/** A checkbox with its label: the shadcn/ui Checkbox, 20 px so the lists' targets keep 24 px apart (WCAG 2.5.8, IR360). */
export function Check({ label, checked, onChange, disabled }: { label: React.ReactNode; checked?: boolean; onChange?: (v: boolean) => void; disabled?: boolean }) {
  return (
    <label className="inline-flex items-center gap-2 text-[13px]">
      <Checkbox checked={checked} disabled={disabled} onCheckedChange={(v) => onChange?.(v === true)} className="size-5" />
      {label}
    </label>
  );
}
/** An on/off switch: the shadcn/ui Switch, named by `label`. */
export function Toggle({ on, onChange, label, disabled }: { on: boolean; onChange: (v: boolean) => void; label?: string; disabled?: boolean }) {
  return <Switch checked={on} onCheckedChange={(v) => onChange(v)} aria-label={label} disabled={disabled} />;
}
/** One choice of a few, shown as pressed buttons: the shadcn/ui ToggleGroup. The chosen one stays chosen when pressed again. */
export function Choice<T extends string>({ options, value, onChange }: { options: { id: T; label: string }[]; value: T; onChange: (v: T) => void }) {
  return (
    <ToggleGroup value={[value]} onValueChange={(v) => { const next = v[v.length - 1] as T | undefined; if (next !== undefined && next !== value) onChange(next); }} variant="outline" className="flex-wrap">
      {options.map((o) => <ToggleGroupItem key={o.id} value={o.id} className="px-3 text-[13px] font-semibold data-pressed:border-primary data-pressed:bg-primary-soft data-pressed:text-primary">{o.label}</ToggleGroupItem>)}
    </ToggleGroup>
  );
}
export function Search({ placeholder, value, onChange }: { placeholder: string; value: string; onChange: (v: string) => void }) {
  return (
    <span className="relative flex w-full min-w-0 items-center">
      <Icon name="search" className="pointer-events-none absolute left-3 text-muted-foreground" />
      <Input type="search" placeholder={placeholder} value={value} onChange={(e) => onChange(e.target.value)} className="pl-8" />
    </span>
  );
}

/** A select for long lists (DD-A03 search-selects): typing narrows the options, the first `limit` matches are listed,
 * the arrow keys and Enter choose one and Escape closes. Closed, it shows the chosen option. An ARIA 1.2 combobox with
 * a listbox popup; `label` names it. */
export function SearchSelect({ value, onChange, options, label, placeholder, limit = 20, className }: {
  value: string; onChange: (id: string) => void; options: { id: string; label: string }[]; label: string; placeholder?: string; limit?: number; className?: string;
}) {
  const t = useT();
  const id = useId();
  const [open, setOpen] = useState(false);
  const [q, setQ] = useState("");
  const [active, setActive] = useState(0);
  const current = options.find((o) => o.id === value);
  const needle = q.trim().toLowerCase();
  const matches = needle ? options.filter((o) => o.label.toLowerCase().includes(needle)) : options;
  const shown = matches.slice(0, limit);
  const close = () => { setOpen(false); setQ(""); };
  const choose = (o: { id: string }) => { close(); if (o.id !== value) onChange(o.id); };
  const start = () => { setOpen(true); setQ(""); setActive(Math.max(0, options.slice(0, limit).findIndex((o) => o.id === value))); };
  return (
    <div className={cx("relative min-w-0", className)}>
      <UIInput
        role="combobox" aria-label={label} aria-expanded={open} aria-controls={`${id}-list`} aria-autocomplete="list" aria-activedescendant={open && shown[active] ? `${id}-${active}` : undefined}
        value={open ? q : current?.label ?? ""} placeholder={placeholder ?? t("Type to search…")} onFocus={start} onBlur={close}
        onChange={(e) => { setQ(e.target.value); setActive(0); setOpen(true); }}
        onKeyDown={(e) => {
          if (e.key === "ArrowDown") { e.preventDefault(); if (!open) start(); else setActive((a) => Math.min(a + 1, shown.length - 1)); }
          else if (e.key === "ArrowUp") { e.preventDefault(); setActive((a) => Math.max(a - 1, 0)); }
          else if (e.key === "Enter" && open && shown[active]) { e.preventDefault(); choose(shown[active]); }
          else if (e.key === "Escape" && open) { e.preventDefault(); close(); }
        }}
        className="pr-8"
      />
      <Icon name="chevronDown" className="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground" />
      {open && (
        <ul id={`${id}-list`} role="listbox" aria-label={label} className="absolute z-30 mt-1 max-h-72 w-full min-w-[220px] overflow-auto rounded-control border border-line bg-surface py-1 shadow-lg">
          {shown.map((o, k) => (
            <li key={o.id || "all"} id={`${id}-${k}`} role="option" aria-selected={o.id === value} onMouseDown={(e) => { e.preventDefault(); choose(o); }} onMouseEnter={() => setActive(k)}
              className={cx("cursor-pointer px-3 py-1.5 text-[13px]", k === active && "bg-primary-soft", o.id === value && "font-semibold")}>{o.label}</li>
          ))}
          {shown.length === 0 && <li className="px-3 py-1.5 text-xs text-muted-foreground">{t("No match — try another name")}</li>}
          {matches.length > shown.length && <li className="px-3 py-1.5 text-[11px] text-muted-foreground">{t("{n} more — type to narrow the list", { n: matches.length - shown.length })}</li>}
        </ul>
      )}
    </div>
  );
}

/* ───────────── Modal ───────────── */
const FOCUSABLE = 'a[href], button:not([disabled]), input:not([disabled]):not([type="hidden"]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

/** A modal dialog: the shadcn/ui Dialog, as the WAI-ARIA dialog pattern and UX-06 ask (IR361). On open, focus moves into
 * it: the first field or control of the body, else the first footer button (Cancel), else the close button. Tab stays
 * inside, Escape or a press outside closes it, and focus returns to what had it before (the trigger). */
export function Modal({ open, title, onClose, children, footer, wide }: { open: boolean; title: string; onClose: () => void; children: React.ReactNode; footer?: React.ReactNode; wide?: boolean }) {
  const t = useT();
  const popup = useRef<HTMLDivElement>(null);
  const firstStop = () => {
    const stop = (part: string) => [...(popup.current?.querySelectorAll<HTMLElement>(`[data-part="${part}"] :is(${FOCUSABLE})`) ?? [])]
      .find((el) => !(el as HTMLButtonElement).disabled && el.tabIndex >= 0);
    return stop("body") ?? stop("footer") ?? stop("header") ?? true;
  };
  return (
    <Dialog open={open} onOpenChange={(next) => { if (!next) onClose(); }}>
      <DialogContent ref={popup} showCloseButton={false} initialFocus={firstStop} className={cn("flex max-h-[92dvh] flex-col gap-0 p-0", wide ? "sm:max-w-3xl" : "sm:max-w-[560px]")}>
        <div data-part="header" className="flex items-center justify-between gap-3 border-b px-5 py-4">
          <DialogTitle className="text-base font-semibold">{title}</DialogTitle>
          <DialogClose render={<Button variant="ghost" size="icon-sm" aria-label={t("Close")} />}><Icon name="close" /></DialogClose>
        </div>
        <div data-part="body" className="flex flex-col gap-3 overflow-y-auto px-5 py-4">{children}</div>
        {footer && <div data-part="footer" className="flex flex-wrap justify-end gap-2 rounded-b-xl border-t bg-muted/50 px-5 py-3">{footer}</div>}
      </DialogContent>
    </Dialog>
  );
}

/* Modal with Cancel + confirm, controlled by a single trigger */
export function useModal() {
  const [open, setOpen] = useState(false);
  return { open, show: () => setOpen(true), hide: () => setOpen(false), setOpen };
}

/* ───────────── Toast ───────────── */
const ToastCtx = createContext<(msg: string, tone?: Tone) => void>(() => {});
export const useToast = () => useContext(ToastCtx);
export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [items, setItems] = useState<{ id: number; msg: string; tone: Tone }[]>([]);
  const push = useCallback((msg: string, tone: Tone = "ok") => {
    const id = Date.now() + Math.random();
    setItems((s) => [...s, { id, msg, tone }]);
    setTimeout(() => setItems((s) => s.filter((x) => x.id !== id)), 3600);
  }, []);
  return (
    <ToastCtx.Provider value={push}>
      {children}
      {/* errors are announced at once (an alert, announced when it appears), other results politely through a status
          region that is always there (UX-06, IR362) */}
      <div className="pointer-events-none fixed inset-x-0 bottom-4 z-[60] flex flex-col items-center gap-2 px-4">
        {(["alert", "status"] as const).filter((role) => role === "status" || items.some((t) => t.tone === "crit")).map((role) => (
          <div key={role} role={role} className="flex flex-col items-center gap-2">
            {items.filter((t) => (t.tone === "crit") === (role === "alert")).map((t) => (
              <div key={t.id} className={cx("pointer-events-auto max-w-md rounded-xl px-4 py-2.5 text-[13px] font-semibold shadow-lg", t.tone === "crit" ? "bg-crit text-white" : t.tone === "warn" ? "bg-warn text-white" : "bg-ink text-white")}>
                {t.msg}
              </div>
            ))}
          </div>
        ))}
      </div>
    </ToastCtx.Provider>
  );
}

/* ───────────── Data display ───────────── */
export type Col<T> = { key: string; label: string; render: (r: T) => React.ReactNode; className?: string; hideBelow?: "sm" | "md" };
/** Responsive table: scrolls horizontally if needed; optional columns hide on narrow containers. A row that opens a
 * record (`onRowClick`) is focusable and opens with Enter or Space as well as a click (keyboard access, IR294). */
export function DataTable<T>({ cols, rows, rowKey, onRowClick, selectedKey }: { cols: Col<T>[]; rows: T[]; rowKey: (r: T) => string; onRowClick?: (r: T) => void; selectedKey?: string }) {
  const t = useT();
  return (
    <div className="scroll-x rounded-xl border border-line">
      <table className="w-full border-collapse text-left text-[13px]">
        <thead>
          <tr className="bg-surface2 text-[11px] uppercase tracking-wide text-muted-foreground">
            {cols.map((c) => (
              <th key={c.key} className={cx("whitespace-nowrap px-3 py-2 font-semibold", c.hideBelow === "sm" && "max-sm:hidden", c.hideBelow === "md" && "max-md:hidden", c.className)}>{c.label}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((r) => {
            const k = rowKey(r);
            return (
              <tr key={k} onClick={onRowClick && (() => onRowClick(r))} tabIndex={onRowClick ? 0 : undefined} aria-current={selectedKey === k ? "true" : undefined}
                onKeyDown={onRowClick && ((e) => { if (e.target === e.currentTarget && (e.key === "Enter" || e.key === " ")) { e.preventDefault(); onRowClick(r); } })}
                className={cx("border-t border-line", onRowClick && "cursor-pointer outline-none hover:bg-surface2/60 focus-visible:bg-surface2/60 focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-primary", selectedKey === k && "bg-primary-soft/60")}>
                {cols.map((c) => (
                  <td key={c.key} className={cx("px-3 py-2.5 align-middle", c.hideBelow === "sm" && "max-sm:hidden", c.hideBelow === "md" && "max-md:hidden", c.className)}>{c.render(r)}</td>
                ))}
              </tr>
            );
          })}
          {rows.length === 0 && (
            <tr><td colSpan={cols.length} className="px-3 py-6 text-center text-muted-foreground">{t("No data")}</td></tr>
          )}
        </tbody>
      </table>
    </div>
  );
}

/** Selectable list row used by master/detail panes. */
export function ListRow({ selected, onClick, href, children, className }: { selected?: boolean; onClick?: () => void; href?: string; children: React.ReactNode; className?: string }) {
  const cls = cx("flex w-full min-w-0 items-center gap-3 rounded-xl border px-3 py-2.5 text-left transition-colors", selected ? "border-primary bg-primary-soft/60" : "border-line bg-surface hover:bg-surface2/70", className);
  if (href) return <Link href={href} className={cls}>{children}</Link>;
  return <button type="button" onClick={onClick} aria-pressed={selected} className={cls}>{children}</button>;
}

/* ───────────── Charts (responsive SVG; scale with container width) ───────────── */
export function BarChart({ labels, series, unit, height = 160, colors = ["#c9d8ee", "#005bea"] }: { labels: string[]; series: number[][]; unit?: string; height?: number; colors?: string[] }) {
  const max = Math.max(1, ...series.flat()) * 1.1;
  const W = 600;
  const pad = { l: 28, r: 4, t: 8, b: 20 };
  const bw = (W - pad.l - pad.r) / labels.length;
  const gw = bw * 0.7 / series.length;
  return (
    <svg viewBox={`0 0 ${W} ${height}`} className="w-full" style={{ height: "auto", aspectRatio: `${W}/${height}` }} role="img" aria-label="Bar chart">
      {[0, 0.5, 1].map((f) => (
        <g key={f}>
          <line x1={pad.l} x2={W} y1={pad.t + (height - pad.t - pad.b) * (1 - f)} y2={pad.t + (height - pad.t - pad.b) * (1 - f)} stroke="#e3edf8" />
          <text x={0} y={pad.t + (height - pad.t - pad.b) * (1 - f) + 4} fontSize="10" fill="#64788e">{Math.round(max * f)}</text>
        </g>
      ))}
      {labels.map((l, i) => (
        <g key={l}>
          {series.map((s, j) => {
            const h = ((height - pad.t - pad.b) * s[i]) / max;
            return <rect key={j} x={pad.l + i * bw + bw * 0.15 + j * gw} y={height - pad.b - h} width={gw - 1} height={h} rx={2} fill={colors[j % colors.length]} />;
          })}
          <text x={pad.l + i * bw + bw / 2} y={height - 5} fontSize="10" textAnchor="middle" fill="#5b6e84">{l}</text>
        </g>
      ))}
      {unit && <text x={W - 2} y={10} fontSize="10" textAnchor="end" fill="#64788e">{unit}</text>}
    </svg>
  );
}

/** A line chart in a viewBox of `width` × `height` units scaled to the container width (a wider viewBox keeps the text
 * small on wide cards); null points are gaps. */
export function LineChart({ points, min, max, threshold, height = 120, width = 600, color = "#005bea", labels, ticks, shadeGaps }: { points: (number | null)[]; min?: number; max?: number; threshold?: number; height?: number; width?: number; color?: string; labels?: string[]; ticks?: number[]; shadeGaps?: boolean }) {
  const W = width;
  const vals = points.filter((p): p is number => p !== null);
  const lo = min ?? Math.min(...vals, threshold ?? Infinity);
  const hi = max ?? Math.max(...vals, threshold ?? -Infinity);
  const y = (v: number) => 8 + (height - 28) * (1 - (v - lo) / (hi - lo || 1));
  const step = (W - 12) / Math.max(1, points.length - 1);
  const x = (i: number) => 6 + step * i;
  // gaps (null) break the line — missing data is never connected; a value between two gaps is drawn as a dot
  const runs: number[][] = [];
  const gaps: [number, number][] = [];
  let cur: number[] = [];
  points.forEach((p, i) => {
    if (p === null) {
      if (cur.length) runs.push(cur);
      cur = [];
      if (gaps.length && gaps[gaps.length - 1][1] === i - 1) gaps[gaps.length - 1][1] = i;
      else gaps.push([i, i]);
      return;
    }
    cur.push(i);
  });
  if (cur.length) runs.push(cur);
  const clampX = (v: number) => Math.min(W - 6, Math.max(6, v));
  return (
    <svg viewBox={`0 0 ${W} ${height}`} className="w-full" style={{ height: "auto", aspectRatio: `${W}/${height}` }} role="img" aria-label="Line chart">
      {shadeGaps && vals.length > 0 && gaps.map(([a, b]) => <rect key={a} x={clampX(x(a) - step / 2)} y={4} width={Math.max(1, clampX(x(b) + step / 2) - clampX(x(a) - step / 2))} height={height - 24} fill="#94a3b8" opacity={0.14} />)}
      {ticks?.map((t) => <g key={t}><line x1={0} x2={W} y1={y(t)} y2={y(t)} stroke="#e2e8f0" /><text x={2} y={y(t) - 2} fontSize="9" fill="#5b6e84">{t}</text></g>)}
      {threshold !== undefined && <line x1={0} x2={W} y1={y(threshold)} y2={y(threshold)} stroke="#dc2626" strokeDasharray="4 3" />}
      {runs.map((r, i) => r.length === 1 ? <circle key={i} cx={x(r[0])} cy={y(points[r[0]]!)} r={3} fill={color} />
        : <path key={i} d={r.map((j, k) => `${k ? "L" : "M"}${x(j).toFixed(1)},${y(points[j]!).toFixed(1)}`).join("")} fill="none" stroke={color} strokeWidth={2} strokeLinejoin="round" />)}
      {labels?.map((l, i) => <text key={i} x={6 + ((W - 12) * i) / Math.max(1, labels.length - 1)} y={height - 4} fontSize="10" fill="#5b6e84" textAnchor={i === 0 ? "start" : i === labels.length - 1 ? "end" : "middle"}>{l}</text>)}
    </svg>
  );
}

/** A progress bar; `label` names it for assistive technology (WCAG 4.1.2, IR359). */
export function UtilBar({ pct, tone = "primary", label }: { pct: number | null; tone?: Tone; label?: string }) {
  return (
    <div className="h-2 w-full overflow-hidden rounded-full bg-surface2" role="progressbar" aria-label={label} aria-valuemin={0} aria-valuemax={100} aria-valuenow={pct ?? undefined}>
      <div className={cx("h-full rounded-full", tone === "ok" ? "bg-ok" : tone === "warn" ? "bg-warn" : tone === "crit" ? "bg-crit" : "bg-primary")} style={{ width: `${pct ?? 0}%` }} />
    </div>
  );
}

/* Form-in-modal helper: required-field validation */
export function useRequired<T extends Record<string, string>>(init: T) {
  const [v, setV] = useState<T>(init);
  const [submitted, setSubmitted] = useState(false);
  const err = (k: keyof T, msg: string, min = 1) => (submitted && v[k].trim().length < min ? msg : undefined);
  return { v, set: (k: keyof T, val: string) => setV((s) => ({ ...s, [k]: val })), submitted, submit: () => setSubmitted(true), reset: () => { setV(init); setSubmitted(false); }, err };
}
