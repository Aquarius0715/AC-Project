"use client";

import { hasContent } from "@ac/web/lib/hasContent";
import Link from "next/link";
import { Children, cloneElement, createContext, isValidElement, useCallback, useContext, useEffect, useRef, useState } from "react";
import { useT } from "@ac/web/components/I18n";
import type { T as Translate } from "@ac/web/lib/i18n";
import type { PickerOption } from "@ac/web/lib/pickerOption";
import { Icon, type IconName } from "@ac/web/components/Icon";
import { Alert } from "@ac/web/components/ui/alert";
import { Badge as UIBadge } from "@ac/web/components/ui/badge";
import { Button, buttonVariants } from "@ac/web/components/ui/button";
import { Card as UICard, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from "@ac/web/components/ui/card";
import { ChartContainer, ChartLegend, ChartLegendContent, ChartTooltip, ChartTooltipContent, type ChartConfig } from "@ac/web/components/ui/chart";
import { Checkbox } from "@ac/web/components/ui/checkbox";
import { Combobox, ComboboxContent, ComboboxEmpty, ComboboxInput, ComboboxItem, ComboboxList } from "@ac/web/components/ui/combobox";
import { Dialog, DialogClose, DialogContent, DialogTitle } from "@ac/web/components/ui/dialog";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@ac/web/components/ui/dropdown-menu";
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@ac/web/components/ui/empty";
import { Input as UIInput } from "@ac/web/components/ui/input";
import { InputGroupAddon } from "@ac/web/components/ui/input-group";
import { NativeSelect } from "@ac/web/components/ui/native-select";
import { Progress } from "@ac/web/components/ui/progress";
import { Switch } from "@ac/web/components/ui/switch";
import { DateTimeSelect, TimeSelect } from "@ac/web/components/TimeSelect";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@ac/web/components/ui/table";
import { Tabs as UITabs, TabsList, TabsTrigger } from "@ac/web/components/ui/tabs";
import { Toaster } from "@ac/web/components/ui/sonner";
import { Textarea as UITextarea } from "@ac/web/components/ui/textarea";
import { ToggleGroup, ToggleGroupItem } from "@ac/web/components/ui/toggle-group";
import { cn } from "@ac/web/lib/utils";
import { tableFeatures, useTable, type ColumnDef, type RowData } from "@tanstack/react-table";
import { axisRange, axisTicks } from "@ac/web/lib/chartScale";
import { Bar, BarChart as RBarChart, CartesianGrid, Line, LineChart as RLineChart, ReferenceArea, ReferenceLine, XAxis, YAxis } from "recharts";
import { toast } from "sonner";

// The product's components (DEC-75): the screens' names (Btn, Card, Badge, …) built on the shadcn/ui components of
// components/ui/. A later phase moves the screens to the shadcn components themselves.

/** The class helper of the screens: shadcn/ui's cn, so a later class wins over a conflicting earlier one (w-auto over w-full). */
export const cx = cn;

/* ───────────── Badges ───────────── */
export type Tone = "ok" | "warn" | "crit" | "unknown" | "primary" | "muted";
function toneClass(tone: Tone): string {
  switch (tone) {
    case "ok": { return "bg-ok-soft text-ok"; }
    case "warn": { return "bg-warn-soft text-warn"; }
    case "crit": { return "bg-crit-soft text-crit"; }
    case "unknown": { return "bg-unknown-soft text-unknown"; }
    case "primary": { return "bg-primary-soft text-primary"; }
    case "muted": { return "bg-surface2 text-muted-foreground"; }
  }
}

/** A status badge: the shadcn/ui Badge in the tone's colours. */
export function Badge({ tone = "muted", icon, children, className }: Readonly<{ tone?: Tone; icon?: IconName; children: React.ReactNode; className?: string }>) {
  return (
    <UIBadge variant="secondary" className={cn(toneClass(tone), className)}>
      {icon && <Icon name={icon} />}
      {children}
    </UIBadge>
  );
}

// the state badges speak the display language (IR258); the state values themselves stay as stored
export function PowerBadge({ s }: Readonly<{ s: "running" | "stopped" | "unknown" }>) {
  const t = useT();
  switch (s) {
    case "running": { return <Badge tone="ok" icon="running">{t("Running")}</Badge>; }
    case "stopped": { return <Badge tone="muted" icon="stopped">{t("Stopped")}</Badge>; }
    case "unknown": { return <Badge tone="unknown" icon="unknown">{t("Unknown")}</Badge>; }
  }
}
export function ConnBadge({ s }: Readonly<{ s: "online" | "offline" | "connecting" | "unknown" | "error" }>) {
  const t = useT();
  switch (s) {
    case "online": { return <Badge tone="ok" icon="online">{t("Online")}</Badge>; }
    case "offline": { return <Badge tone="unknown" icon="offline">{t("Offline")}</Badge>; }
    case "error": { return <Badge tone="crit" icon="error">{t("Error")}</Badge>; }
    case "unknown": { return <Badge tone="unknown" icon="unknown">{t("Unknown")}</Badge>; }
    case "connecting": { return <Badge tone="primary" icon="refresh">{t("Connecting")}</Badge>; }
  }
}
export function SeverityBadge({ s }: Readonly<{ s: "critical" | "warning" | "normal" }>) {
  const t = useT();
  switch (s) {
    case "critical": { return <Badge tone="crit" icon="critical">{t("Critical")}</Badge>; }
    case "warning": { return <Badge tone="warn" icon="warning">{t("Warning")}</Badge>; }
    case "normal": { return <Badge tone="primary" icon="info">{t("Info")}</Badge>; }
  }
}
export function OnOffBadge({ on }: { on: boolean }) {
  const t = useT();
  return on ? <Badge tone="ok">{t("On")}</Badge> : <Badge tone="muted">{t("Off")}</Badge>;
}
export const DemoBadge = () => <Badge tone="warn" className="uppercase tracking-wide">Demo</Badge>;

/* ───────────── Buttons ───────────── */
type ButtonVariant = "primary" | "secondary" | "ghost" | "danger";
// the product's button names on the shadcn/ui Button variants; ghost keeps the primary colour of a text action
type ShadcnVariant = "default" | "outline" | "ghost" | "destructive";
function variantOf(v: ButtonVariant): ShadcnVariant {
  switch (v) {
    case "primary": { return "default"; }
    case "secondary": { return "outline"; }
    case "ghost": { return "ghost"; }
    case "danger": { return "destructive"; }
  }
}
const variantCls = (v: ButtonVariant) => cn(v === "secondary" && "bg-card", v === "ghost" && "text-primary hover:bg-accent hover:text-primary");
const sizeOf = (size: "sm" | "md") => (size === "sm" ? "sm" : "default");
const buttonClass = (v: ButtonVariant, size: "sm" | "md") => cn(buttonVariants({ variant: variantOf(v), size: sizeOf(size) }), variantCls(v));

export function Btn({ variant = "secondary", size = "md", className, ...p }: React.ButtonHTMLAttributes<HTMLButtonElement> & { variant?: ButtonVariant; size?: "sm" | "md" }) {
  return <Button type="button" {...p} variant={variantOf(variant)} size={sizeOf(size)} className={cn(variantCls(variant), className)} />;
}
export function LinkButton({ href, variant = "secondary", size = "md", className, children }: Readonly<{ href: string; variant?: ButtonVariant; size?: "sm" | "md"; className?: string; children: React.ReactNode }>) {
  return <Link href={href} className={cx(buttonClass(variant, size), className)}>{children}</Link>;
}
export function TextLink({ href, children, className }: Readonly<{ href: string; children: React.ReactNode; className?: string }>) {
  return <Link href={href} className={cx("text-xs font-semibold text-primary hover:underline", className)}>{children}</Link>;
}

/* ───────────── Layout primitives ───────────── */
/** The main column: 1280 px wide, or the 640 px single-column of offers and snapshots (Figma) with `narrow`. */
export function Page({ children, className, narrow }: Readonly<{ children: React.ReactNode; className?: string; narrow?: boolean }>) {
  return <div className={cx("page mx-auto flex w-full flex-col gap-4", narrow ? "max-w-[640px]" : "max-w-[1280px]", className)}>{children}</div>;
}

/** A card: the shadcn/ui Card. The title stays a level-2 heading; `pad={false}` lets a table reach the edges; the
overflow stays visible for the menus that still open inside a card. */
export function Card({ title, sub, action, children, className, pad = true, tone }: Readonly<{ title?: React.ReactNode; sub?: React.ReactNode; action?: React.ReactNode; children?: React.ReactNode; className?: string; pad?: boolean; tone?: "crit" | "warn" | "ok" }>) {
  return (
    <UICard className={cn("min-w-0 overflow-visible", !pad && "pb-0", tone === "crit" && "ring-crit", tone === "warn" && "ring-[#fdba74]", tone === "ok" && "ring-[#86efac]", className)}>
      {(Boolean(title) || Boolean(action)) && (
        // the title keeps at least 12rem: a wide action (a row of filters) goes below it on a narrow card instead of
        // squeezing it into a column; a small one stays beside it
        <CardHeader className="flex flex-wrap items-start justify-between gap-x-3 gap-y-2">
          <div className="flex min-w-0 flex-[1_1_12rem] flex-col gap-1.5">
            {hasContent(title) && <CardTitle><h2>{title}</h2></CardTitle>}
            {hasContent(sub) && <CardDescription className="text-xs">{sub}</CardDescription>}
          </div>
          {hasContent(action) && <CardAction className="self-start">{action}</CardAction>}
        </CardHeader>
      )}
      <CardContent className={cn("min-w-0", !pad && "px-0")}>{children}</CardContent>
    </UICard>
  );
}

export function PageHead({ title, sub, action }: Readonly<{ title: React.ReactNode; sub?: React.ReactNode; action?: React.ReactNode }>) {
  return (
    <div className="flex flex-wrap items-end justify-between gap-3">
      <div className="min-w-0">
        <h1 className="text-lg font-bold text-ink">{title}</h1>
        {hasContent(sub) && <p className="text-xs text-muted-foreground">{sub}</p>}
      </div>
      {action}
    </div>
  );
}

/** A part that scrolls sideways when the screen is narrower than its content (a wide table, a board). While it overflows
it can take the focus, so a keyboard can scroll it (WCAG 2.1.1, axe scrollable-region-focusable); with a label it is a
named region. On a wide screen nothing overflows and it adds no tab stop. */
export function ScrollX({ label, className, children }: Readonly<{ label?: string; className?: string; children: React.ReactNode }>) {
  const ref = useRef<HTMLDivElement>(null);
  const [overflows, setOverflows] = useState(false);
  useEffect(() => {
    const el = ref.current;
    const check = () => { if (el) setOverflows(el.scrollWidth > el.clientWidth + 1); };
    const observer = new ResizeObserver(check);
    if (el) {
      check();
      observer.observe(el);
      for (const child of el.children) observer.observe(child);
    }
    return () => { observer.disconnect(); };
  }, []);
  // relative: an absolutely positioned descendant (a visually hidden column header) is clipped with the table instead
  // of widening the page (IR501)
  return <div ref={ref} className={cx("scroll-x relative", className)} tabIndex={overflows ? 0 : undefined} role={label ? "region" : undefined} aria-label={label}>{children}</div>;
}
export function Kpi({ label, value, icon, unit, badge, sub, href, link, tone }: Readonly<{ label: string; value: React.ReactNode; icon?: IconName; unit?: string; badge?: React.ReactNode; sub?: React.ReactNode; href?: string; link?: string; tone?: "crit" | "warn" | "ok" }>) {
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
      {/* a long word without spaces (unknown/connecting/error) breaks instead of running out of a narrow card */}
      {hasContent(sub) && <p className="break-words text-xs text-muted-foreground">{sub}</p>}
      {href && link && <TextLink href={href} className="mt-auto pt-1">{link} <Icon name="arrowRight" /></TextLink>}
    </UICard>
  );
}

export function Stat({ label, value, sub }: Readonly<{ label: string; value: React.ReactNode; sub?: React.ReactNode }>) {
  return (
    <div className="min-w-0 rounded-xl bg-surface2 p-3">
      <div className="text-[11px] text-muted-foreground">{label}</div>
      <div className="text-base font-bold text-ink">{value}</div>
      {hasContent(sub) && <div className="text-[11px] text-muted-foreground">{sub}</div>}
    </div>
  );
}

// a banner's border and background, and its icon, per tone (primary for the others)
const BANNER_CLS = new Map<Tone, string>([["warn", "border-[#fdba74] bg-warn-soft/60"], ["crit", "border-crit bg-crit-soft/60"], ["ok", "border-[#86efac] bg-ok-soft/60"]]);
const BANNER_ICON = new Map<Tone, IconName>([["warn", "warning"], ["crit", "error"], ["ok", "success"]]);
/** A message line: the shadcn/ui Alert in the tone's colours, an alert only when critical (otherwise a status). */
export function Banner({ tone = "primary", icon, children, action }: Readonly<{ tone?: Tone; icon?: IconName; children: React.ReactNode; action?: React.ReactNode }>) {
  const bd = BANNER_CLS.get(tone) ?? "border-border bg-primary-soft/50";
  return (
    <Alert role={tone === "crit" ? "alert" : "status"} className={cn("flex flex-wrap items-center gap-x-3 gap-y-2 rounded-xl px-3.5 py-2.5 text-[13px] *:[svg]:translate-y-0", bd)}>
      <Icon name={icon ?? BANNER_ICON.get(tone) ?? "info"} className="text-[15px]" />
      {/* the text keeps at least 15rem: on a narrow screen the action goes below it instead of squeezing it */}
      <div className="min-w-0 flex-[1_1_15rem]">{children}</div>
      {action}
    </Alert>
  );
}

/** No rows, a failed read, no connection: the shadcn/ui Empty. The title stays a level-3 heading. */
export function EmptyState({ title, children, action }: Readonly<{ title: string; children?: React.ReactNode; action?: React.ReactNode }>) {
  return (
    <Empty className="border bg-card">
      <EmptyHeader>
        <EmptyMedia variant="icon"><Icon name="info" /></EmptyMedia>
        <EmptyTitle><h3>{title}</h3></EmptyTitle>
        {hasContent(children) && <EmptyDescription>{children}</EmptyDescription>}
      </EmptyHeader>
      {hasContent(action) && <EmptyContent>{action}</EmptyContent>}
    </Empty>
  );
}
export function ErrorState({ title, children, onRetry }: Readonly<{ title: string; children?: React.ReactNode; onRetry?: () => void }>) {
  return (
    <Empty className="border border-solid border-crit bg-card">
      <EmptyHeader>
        <EmptyMedia variant="icon" className="bg-crit-soft text-crit"><Icon name="error" /></EmptyMedia>
        <EmptyTitle className="text-crit"><h3>{title}</h3></EmptyTitle>
        {hasContent(children) && <EmptyDescription>{children}</EmptyDescription>}
      </EmptyHeader>
      <EmptyContent><Btn size="sm" onClick={onRetry}><Icon name="refresh" /> Retry</Btn></EmptyContent>
    </Empty>
  );
}

export function SummaryList({ items, cols = 1 }: Readonly<{ items: [string, React.ReactNode][]; cols?: 1 | 2 }>) {
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

// a step's text and its number disc: the current one, the done ones and the rest
const STEP_CLS = { current: ["text-primary", "bg-primary text-white"], done: ["text-ok", "bg-ok-soft"], todo: ["text-muted-foreground", "bg-surface2"] } as const;
const stepCls = (i: number, current: number) => {
  if (i === current) return STEP_CLS.current;
  return i < current ? STEP_CLS.done : STEP_CLS.todo;
};
export function Steps({ steps, current }: Readonly<{ steps: string[]; current: number }>) {
  return (
    <ol className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs font-semibold">
      {steps.map((s, i) => (
        <li key={s} className={cx("flex items-center gap-1.5", stepCls(i, current)[0])}>
          <span className={cx("grid h-5 w-5 place-items-center rounded-full text-[11px]", stepCls(i, current)[1])}>{i < current ? <Icon name="check" /> : i + 1}</span>
          {s}
        </li>
      ))}
    </ol>
  );
}

const DOT_CLS = new Map<Tone, string>([["crit", "bg-crit"], ["warn", "bg-warn"], ["ok", "bg-ok"]]);
export function Timeline({ items }: Readonly<{ items: { time: string; title: React.ReactNode; detail?: React.ReactNode; tone?: Tone }[] }>) {
  return (
    <ol className="flex flex-col gap-3">
      {items.map((it, i) => (
        <li key={i} className="grid grid-cols-[84px_14px_1fr] items-start gap-2 text-[13px]">
          <span className="text-xs text-muted-foreground">{it.time}</span>
          <span className={cx("mt-1.5 h-2.5 w-2.5 rounded-full", (it.tone && DOT_CLS.get(it.tone)) ?? "bg-primary")} />
          <div className="min-w-0">
            <div className="font-semibold">{it.title}</div>
            {hasContent(it.detail) && <div className="text-xs text-muted-foreground">{it.detail}</div>}
          </div>
        </li>
      ))}
    </ol>
  );
}

/* ───────────── Tabs / segmented ───────────── */
/** A tab bar: the shadcn/ui Tabs (tablist and tabs; the screens draw the panels). `count` is a badge after the label. */
export function Tabs<T extends string>({ tabs, value, onChange, className }: Readonly<{ tabs: { id: T; label: string; count?: number | string }[]; value: T; onChange: (v: T) => void; className?: string }>) {
  return (
    <UITabs value={value} onValueChange={(v) => { const tab = tabs.find((x) => x.id === v); if (tab) onChange(tab.id); }} className={cn("max-w-full", className)}>
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
export function Field({ label, hint, error, children, className, labelHidden }: Readonly<{ label: string; hint?: React.ReactNode; error?: React.ReactNode; children: React.ReactNode; className?: string; labelHidden?: boolean }>) {
  // Only a single form control sits inside a <label>. Anything else — choice buttons, a set of checkboxes or buttons, a
  // read-only box — is a labelled group: inside a <label> a button takes the label as its name (IR230, IR238).
  // `labelHidden` keeps the label for assistive technology only, for a group that shows labels of its own.
  const element = isValidElement<{ "aria-invalid"?: boolean }>(children) ? children : null;
  const isControl = element !== null && (element.type === Input || element.type === Select || element.type === Textarea || (typeof element.type === "string" && ["input", "select", "textarea"].includes(element.type)));
  const isGroup = !isControl;
  const Tag = isGroup ? "div" : "label";
  // an error marks its control invalid, so assistive technology tells which field to fix; the error text is part of
  // the label, and so of the control's name (IR573)
  // the time pickers are groups of controls: an error marks each of their controls invalid the same way (IR573, IR638)
  const isPicker = element !== null && (element.type === TimeSelect || element.type === DateTimeSelect);
  const field = element && (isControl || isPicker) && hasContent(error) ? cloneElement(element, { "aria-invalid": true }) : children;
  return (
    <Tag role={isGroup ? "group" : undefined} aria-label={isGroup ? label : undefined} className={cx("flex min-w-0 flex-col gap-1 text-xs text-ink", className)}>
      <span aria-hidden={isGroup || undefined} className={cx("font-semibold", labelHidden && "sr-only")}>{label}</span>
      {field}
      {hasContent(error) ? <span className="flex items-center gap-1 font-medium text-crit"><Icon name="error" />{error}</span> : null}
      {!hasContent(error) && hasContent(hint) ? <span className="font-normal text-muted-foreground">{hint}</span> : null}
    </Tag>
  );
}
// The form controls are the shadcn/ui Input, Textarea and NativeSelect. A width in className (w-auto, w-40) replaces the
// default one (cn merges), and a select stays a native select, so the platform's own picker works everywhere.
export const Input = ({ className, ...p }: React.ComponentProps<"input">) => <UIInput {...p} className={className} />; // React 19: ref is a prop
/** A value a form sends without showing it (a hidden input). */
export const Hidden = ({ name, value }: Readonly<{ name: string; value: string }>) => <input type="hidden" name={name} value={value} />;
export const Select = ({ className, children, ...p }: Omit<React.SelectHTMLAttributes<HTMLSelectElement>, "size">) => (
    <NativeSelect {...p} className={cn("w-full", className)}>{children}</NativeSelect>
  );
/** A row's actions behind the "more" icon: the shadcn/ui DropdownMenu. The actions are written as <option value>
children (shown only when they apply); `onChoose` gets the chosen value. */
export function ActionMenu({ label, disabled, onChoose, children }: Readonly<{ label: string; disabled?: boolean; onChoose: (value: string) => void; children: React.ReactNode }>) {
  const items = Children.toArray(children).filter((c): c is React.ReactElement<{ value: string; children: React.ReactNode }> => isValidElement(c) && c.type === "option");
  return (
    <DropdownMenu>
      <DropdownMenuTrigger disabled={disabled} render={<Button variant="ghost" size="icon-sm" aria-label={label} className="text-muted-foreground" />}>
        <Icon name="more" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-44">
        {items.map((o) => <DropdownMenuItem key={o.props.value} onClick={() => { onChoose(o.props.value); }}>{o.props.children}</DropdownMenuItem>)}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
export const Textarea = ({ className, ...p }: React.TextareaHTMLAttributes<HTMLTextAreaElement>) => <UITextarea {...p} className={cn("min-h-[84px] resize-y", className)} />;
/** A button with no look of its own, like the browser's button under Tailwind's reset: the text, size and layout come from
its parent and its classes (the 44 px touch height of IR615 too), except that a chip or a tab is touch-sized (IR617). A screen uses it for a selectable card, a chip or a text action, so the shadcn Button's
height, padding, radius and weight are reset here rather than in each screen. */
export const Plain = ({ className, touch, ...p }: React.ComponentProps<typeof Button> & { touch?: boolean }) => {
  // a chip (aria-pressed) or a tab (role tab) is a control of its own: on a touch screen it is 44 px tall like the
  // shared ones, its label centred (IR617); so is an action marked `touch` — an icon or a text action — 44×44 (IR618)
  const isControl = touch === true || p["aria-pressed"] !== undefined || p.role === "tab";
  return <Button {...p} data-plain="" variant="ghost" className={cn("inline-block h-auto min-w-0 pointer-coarse:min-h-0 shrink gap-0 rounded-none p-0 font-[inherit] whitespace-normal justify-start leading-[inherit] [font-size:inherit] [letter-spacing:inherit] active:translate-y-0 disabled:opacity-100 [&_svg:not([class*='size-'])]:size-auto",
    isControl && "pointer-coarse:inline-flex pointer-coarse:min-h-11 pointer-coarse:items-center", touch && "pointer-coarse:min-w-11 pointer-coarse:justify-center", className)} />;
};
/** A one-time code as separate boxes (Figma 10g): one transparent input over the boxes, so typing, pasting and the
browser's one-time-code autofill all work; the box for the next digit is outlined while the input has focus. */
/** A one-time code of `length` digits in boxes over one input. The input has no maxlength: a code is copied as "123 456", and the
browser would cut the pasted text to `length` characters before the digits are picked out (IR678); onChange keeps the digits
and cuts them to `length`. */
export function CodeInput({ value, onChange, label, length = 6 }: Readonly<{ value: string; onChange: (v: string) => void; label: string; length?: number }>) {
  const [focused, setFocused] = useState(false);
  const next = Math.min(value.length, length - 1);
  return (
    <div className="relative inline-flex gap-2">
      {Array.from({ length }, (_, i) => (
          <span key={i} aria-hidden className={cx("grid h-10 w-10 place-items-center rounded-lg border bg-surface text-lg font-semibold", focused && i === next ? "border-primary ring-2 ring-primary/20" : "border-line")}>{value.at(i) ?? ""}</span>
        ))}
      <input aria-label={label} inputMode="numeric" autoComplete="one-time-code" value={value}
        onChange={(e) => { onChange(e.target.value.replaceAll(/\D/g, "").slice(0, length)); }} onFocus={() => { setFocused(true); }} onBlur={() => { setFocused(false); }}
        className="absolute inset-0 h-full w-full cursor-text opacity-0" />
    </div>
  );
}
/** A QR code drawn from its module path (lib/twoFactor qrPath), dark on white whatever the theme so it scans. */
export function QrCode({ qr, label, className }: Readonly<{ qr: { size: number; path: string }; label: string; className?: string }>) {
  return (
    <svg role="img" aria-label={label} viewBox={`0 0 ${String(qr.size)} ${String(qr.size)}`} shapeRendering="crispEdges" className={cx("block bg-white", className)}>
      <path d={qr.path} fill="#0b1220" />
    </svg>
  );
}
/** A checkbox with its label: the shadcn/ui Checkbox, 20 px so the lists' targets keep 24 px apart (WCAG 2.5.8, IR360). */
export function Check({ label, checked, onChange, disabled }: Readonly<{ label: React.ReactNode; checked?: boolean; onChange?: (v: boolean) => void; disabled?: boolean }>) {
  return (
    <label className="inline-flex items-center gap-2 text-[13px]">
      <Checkbox checked={checked} disabled={disabled} onCheckedChange={(v) => onChange?.(v)} className="size-5" />
      {label}
    </label>
  );
}
/** An on/off switch: the shadcn/ui Switch, named by `label`. */
export function Toggle({ on, onChange, label, disabled }: Readonly<{ on: boolean; onChange: (v: boolean) => void; label?: string; disabled?: boolean }>) {
  return <Switch checked={on} onCheckedChange={(v) => { onChange(v); }} aria-label={label} disabled={disabled} />;
}
/** One choice of a few, shown as pressed buttons: the shadcn/ui ToggleGroup. The chosen one stays chosen when pressed again. */
export function Choice<T extends string>({ options, value, onChange, disabled }: Readonly<{ options: { id: T; label: string }[]; value: T; onChange: (v: T) => void; disabled?: boolean }>) {
  return (
    <ToggleGroup value={[value]} disabled={disabled} onValueChange={(v) => { const next = options.find((o) => o.id === v.at(-1)); if (next && next.id !== value) onChange(next.id); }} variant="outline" className="flex-wrap">
      {options.map((o) => <ToggleGroupItem key={o.id} value={o.id} className="px-3 text-[13px] font-semibold data-pressed:border-primary data-pressed:bg-primary-soft data-pressed:text-primary">{o.label}</ToggleGroupItem>)}
    </ToggleGroup>
  );
}
export function Search({ placeholder, value, onChange }: Readonly<{ placeholder: string; value: string; onChange: (v: string) => void }>) {
  return (
    <span className="relative flex w-full min-w-0 items-center">
      <Icon name="search" className="pointer-events-none absolute left-3 text-muted-foreground" />
      <Input type="search" placeholder={placeholder} value={value} onChange={(e) => { onChange(e.target.value); }} className="pl-8" />
    </span>
  );
}

/** A select for long lists (DD-A03 search-selects): the shadcn/ui Combobox. Typing narrows the options, the first
`limit` matches are listed, the arrow keys and Enter choose one and Escape closes; closed, it shows the chosen
option. `label` names the input and its list. */
export function SearchSelect({ value, onChange, options, label, placeholder, limit = 20, className }: Readonly<{
  value: string; onChange: (id: string) => void; options: { id: string; label: string }[]; label: string; placeholder?: string; limit?: number; className?: string;
}>) {
  const t = useT();
  const [query, setQuery] = useState("");
  const current = options.find((o) => o.id === value) ?? null;
  const needle = query.trim().toLowerCase();
  // the chosen option's own label in the input lists everything, as an empty input does
  const matches = !needle || needle === current?.label.toLowerCase() ? options : options.filter((o) => o.label.toLowerCase().includes(needle));
  return (
    <Combobox items={options} filteredItems={matches} limit={limit} value={current} openOnInputClick
      itemToStringLabel={(o: (typeof options)[number]) => o.label} isItemEqualToValue={(a: (typeof options)[number], b: (typeof options)[number]) => a.id === b.id}
      onInputValueChange={(q) => { setQuery(q); }} onValueChange={(o) => { if (o && o.id !== value) onChange(o.id); }}>
      {/* the input opens the list (a click, the arrow keys); the chevron only shows that it is a list */}
      <ComboboxInput aria-label={label} placeholder={placeholder ?? t("Type to search…")} showTrigger={false} className={cn("w-full min-w-0", className)}>
        <InputGroupAddon align="inline-end" aria-hidden><Icon name="chevronDown" className="text-muted-foreground" /></InputGroupAddon>
      </ComboboxInput>
      <ComboboxContent>
        <ComboboxEmpty>{t("No match — try another name")}</ComboboxEmpty>
        <ComboboxList aria-label={label}>{(o: { id: string; label: string }) => <ComboboxItem key={o.id || "all"} value={o}>{o.label}</ComboboxItem>}</ComboboxList>
        {matches.length > limit && <p className="px-3 py-1.5 text-[11px] text-muted-foreground">{t("{n} more — type to narrow the list", { n: matches.length - limit })}</p>}
      </ComboboxContent>
    </Combobox>
  );
}

/** A select for lists too long to read whole (units, devices): typing asks `search` on the server, so every record in
scope can be found, not only the first page. Each option shows its label with a second line of details (customer, place,
model, serial…) and one that cannot be chosen says why. Closed, it shows the chosen option. */
export function AsyncSearchSelect({ value, onChange, search, label, placeholder, className }: Readonly<{
  value: PickerOption | null; onChange: (o: PickerOption | null) => void; search: (query: string) => Promise<PickerOption[]>; label: string; placeholder?: string; className?: string;
}>) {
  const t = useT();
  const [query, setQuery] = useState("");
  const [options, setOptions] = useState<PickerOption[]>([]);
  const [failed, setFailed] = useState(false);
  // a new query asks after a short pause, and an answer that comes after a newer query is dropped
  useEffect(() => {
    let current = true;
    const timer = setTimeout(() => {
      search(query === value?.label ? "" : query).then((found) => {
        if (!current) return;
        setOptions(found);
        setFailed(false);
      }).catch(() => { if (current) setFailed(true); });
    }, 250);
    return () => { current = false; clearTimeout(timer); };
  }, [query, search, value?.label]);
  return (
    <Combobox items={options} filteredItems={options} value={value} openOnInputClick
      itemToStringLabel={(o: PickerOption) => o.label} isItemEqualToValue={(a: PickerOption, b: PickerOption) => a.id === b.id}
      onInputValueChange={(q) => { setQuery(q); }} onValueChange={(o) => { onChange(o); }}>
      <ComboboxInput aria-label={label} placeholder={placeholder ?? t("Type to search…")} showTrigger={false} className={cn("w-full min-w-0", className)}>
        <InputGroupAddon align="inline-end" aria-hidden><Icon name="chevronDown" className="text-muted-foreground" /></InputGroupAddon>
      </ComboboxInput>
      <ComboboxContent>
        <ComboboxEmpty>{t(failed ? "The search failed — try again" : "No match — try another name")}</ComboboxEmpty>
        <ComboboxList aria-label={label}>
          {(o: PickerOption) => (
            <ComboboxItem key={o.id} value={o} disabled={o.blocked !== null}>
              <span className="flex min-w-0 flex-col"><span>{o.label}</span>
                {(o.detail || o.blocked) && <span className="truncate text-[11px] text-muted-foreground">{o.blocked ? t("On {unit} — move it from its page", { unit: o.blocked }) : o.detail}</span>}</span>
            </ComboboxItem>
          )}
        </ComboboxList>
      </ComboboxContent>
    </Combobox>
  );
}

/* ───────────── Modal ───────────── */
const FOCUSABLE = 'a[href], button:not([disabled]), input:not([disabled]):not([type="hidden"]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

/** A modal dialog: the shadcn/ui Dialog, as the WAI-ARIA dialog pattern and UX-06 ask (IR361). On open, focus moves into
it: the first field or control of the body, else the first footer button (Cancel), else the close button. Tab stays
inside, Escape or a press outside closes it, and focus returns to what had it before (the trigger) — or to what
`finalFocus` returns, where the dialog's own action takes the trigger away (a removed row, IR612). That element
itself takes the focus, also a list region outside the Tab order: Base UI would pass it on to the region's first
control, another row's button (IR620). */
export function Modal({ open, title, onClose, children, footer, wide, finalFocus }: Readonly<{ open: boolean; title: string; onClose: () => void; children: React.ReactNode; footer?: React.ReactNode; wide?: boolean; finalFocus?: () => HTMLElement | true | null }>) {
  const t = useT();
  const popup = useRef<HTMLDivElement>(null);
  const firstStop = () => {
    const stop = (part: string) => {
      return [...(popup.current?.querySelectorAll<HTMLElement>(`[data-part="${CSS.escape(part)}"] :is(${FOCUSABLE})`) ?? [])]
        .find((el) => !el.matches(":disabled") && el.tabIndex >= 0);
    };
    return stop("body") ?? stop("footer") ?? stop("header") ?? true;
  };
  const returnTo = finalFocus && (() => {
    const el = finalFocus();
    if (!(el instanceof HTMLElement)) return el ?? true;
    queueMicrotask(() => { el.focus({ preventScroll: true }); });
    return false;
  });
  return (
    <Dialog open={open} onOpenChange={(next) => { if (!next) onClose(); }}>
      <DialogContent ref={popup} showCloseButton={false} initialFocus={firstStop} finalFocus={returnTo ?? true} className={cn("flex max-h-[92dvh] flex-col gap-0 p-0", wide ? "sm:max-w-3xl" : "sm:max-w-[560px]")}>
        <div data-part="header" className="flex items-center justify-between gap-3 border-b px-5 py-4">
          <DialogTitle className="text-base font-semibold">{title}</DialogTitle>
          <DialogClose render={<Button variant="ghost" size="icon-sm" aria-label={t("Close")} />}><Icon name="close" /></DialogClose>
        </div>
        <div data-part="body" className="flex flex-col gap-3 overflow-y-auto px-5 py-4">{children}</div>
        {hasContent(footer) && <div data-part="footer" className="flex flex-wrap justify-end gap-2 rounded-b-xl border-t bg-muted/50 px-5 py-3">{footer}</div>}
      </DialogContent>
    </Dialog>
  );
}


/* ───────────── Toast ───────────── */
const ToastCtx = createContext<(notice: string, tone?: Tone) => void>(() => { /* outside a ToastProvider a notice has nowhere to show */ });
export const useToast = () => useContext(ToastCtx);
/** The toasts: shadcn/ui's Sonner. Sonner announces through one polite region, so an error carries its text in an alert
(announced at once) and every other result in a status (UX-06, IR362). */
export function ToastProvider({ children }: Readonly<{ children: React.ReactNode }>) {
  const push = useCallback((notice: string, tone: Tone = "ok") => {
    const body = <span role={tone === "crit" ? "alert" : "status"}>{notice}</span>;
    switch (tone) {
    case "crit": {
    toast.error(body);
    break;
    }
    case "warn": {
    toast.warning(body);
    break;
    }
    case "ok": {
    toast.success(body);
    break;
    }
    default: { toast.info(body);
    }
    }
  }, []);
  return (
    <ToastCtx.Provider value={push}>
      {children}
      <Toaster position="bottom-center" />
    </ToastCtx.Provider>
  );
}

/* ───────────── Data display ───────────── */
export interface Col<T> { key: string; label: string; render: (r: T) => React.ReactNode; className?: string; hideBelow?: "sm" | "md" }
// TanStack Table builds the rows and cells (shadcn/ui's data table, DEC-75); a column carries its classes as meta
// the columns' meta type, given to TanStack through its type-only columnMeta slot
const columnMeta: { className?: string; hideBelow?: "sm" | "md" } = {};
const tableFx = tableFeatures({ columnMeta });
const HIDE_CLS = new Map([["sm", "max-sm:hidden"], ["md", "max-md:hidden"]]);
const hideCls = (m?: { hideBelow?: "sm" | "md" }) => (m?.hideBelow ? HIDE_CLS.get(m.hideBelow) : undefined);
/** A cell's content under one component type for good. A column's `render` is a new function on every render of the
page, and FlexRender mounts a function as a component, so each render remounted every cell: a row's button that
opened a dialog was gone when the dialog closed, and focus fell to the page (IR620). */
function Cell<T>({ render, row }: { render: ((r: T) => React.ReactNode) | undefined; row: T }) {
  return render?.(row) ?? null; // every cell's column has its render; a missing one shows nothing
}
/** Responsive table: scrolls horizontally if needed; optional columns hide on narrow containers. A row that opens a
record (`onRowClick`) is focusable and opens with Enter or Space as well as a click (keyboard access, IR294). */
export function DataTable<T extends RowData>({ cols, rows, rowKey, onRowClick, selectedKey }: Readonly<{ cols: Col<T>[]; rows: T[]; rowKey: (r: T) => string; onRowClick?: (r: T) => void; selectedKey?: string }>) {
  const t = useT();
  const columns = cols.map((c): ColumnDef<typeof tableFx, T> => ({ id: c.key, header: c.label, meta: { className: c.className, hideBelow: c.hideBelow } }));
  const renders = new Map(cols.map((c) => [c.key, c.render]));
  const table = useTable({ features: tableFx, data: rows, getRowId: (r) => rowKey(r), columns });
  return (
    <div className="overflow-hidden rounded-xl border">
      <Table className="text-[13px]">
        <TableHeader>
          {table.getHeaderGroups().map((g) => (
            <TableRow key={g.id} className="bg-muted text-[11px] tracking-wide text-muted-foreground uppercase hover:bg-muted">
              {g.headers.map((h) => (
                <TableHead key={h.id} className={cx("h-auto px-3 py-2 font-semibold text-muted-foreground", hideCls(h.column.columnDef.meta), h.column.columnDef.meta?.className)}>
                  {h.isPlaceholder ? null : <table.FlexRender header={h} />}
                </TableHead>
              ))}
            </TableRow>
          ))}
        </TableHeader>
        <TableBody>
          {table.getRowModel().rows.map((row) => {
            const r = row.original;
            const isSelected = selectedKey === row.id;
            return (
              <TableRow key={row.id} onClick={onRowClick && (() => { onRowClick(r); })} tabIndex={onRowClick ? 0 : undefined} aria-current={isSelected ? "true" : undefined} data-state={isSelected ? "selected" : undefined}
                onKeyDown={onRowClick && ((e) => { if (!(e.target === e.currentTarget && (e.key === "Enter" || e.key === " "))) {
                  return;
                }

                e.preventDefault(); onRowClick(r); })}
                className={cx("data-[state=selected]:bg-primary-soft/60", onRowClick && "cursor-pointer outline-none focus-visible:bg-muted/60 focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-primary")}>
                {row.getAllCells().map((cell) => (
                  <TableCell key={cell.id} className={cx("px-3 py-2.5 whitespace-normal", hideCls(cell.column.columnDef.meta), cell.column.columnDef.meta?.className)}>
                    <Cell render={renders.get(cell.column.id)} row={r} />
                  </TableCell>
                ))}
              </TableRow>
            );
          })}
          {rows.length === 0 && (
            <TableRow className="hover:bg-transparent"><TableCell colSpan={cols.length} className="h-24 text-center text-muted-foreground">{t("No data")}</TableCell></TableRow>
          )}
        </TableBody>
      </Table>
    </div>
  );
}

/** Selectable list row used by master/detail panes. */
export function ListRow({ id, selected, onClick, href, children, className }: { id?: string; selected?: boolean; onClick?: () => void; href?: string; children: React.ReactNode; className?: string }) {
  const cls = cx("flex w-full min-w-0 items-center gap-3 rounded-xl border px-3 py-2.5 text-left transition-colors", selected ? "border-primary bg-primary-soft/60" : "border-line bg-surface hover:bg-surface2/70", className);
  return href ? <Link id={id} href={href} className={cls}>{children}</Link> : <button id={id} type="button" onClick={onClick} aria-pressed={selected} className={cls}>{children}</button>;
}

/* ───────────── Charts: shadcn/ui's chart on Recharts (DEC-75); full width, `height` pixels tall ───────────── */
const axisTick = { fontSize: 10 };
const defaultBarColors = ["#c9d8ee", "#005bea"];

/** Grouped bars, one group per label. `names` name the series in the tooltip and the legend under the chart. */
export function BarChart({ labels, series, names, unit, height = 160, colors = defaultBarColors }: Readonly<{ labels: string[]; series: number[][]; names?: string[]; unit?: string; height?: number; colors?: string[] }>) {
  const t = useT();
  const max = Math.max(1, ...series.flat()) * 1.1;
  const data = labels.map((label, i) => ({ label, ...Object.fromEntries(series.map((s, j) => [`s${String(j)}`, s.at(i)])) }));
  const config: ChartConfig = Object.fromEntries(series.map((_, j) => [`s${String(j)}`, { label: names?.at(j) ?? t("Value"), color: colors.at(j % colors.length) }]));
  return (
    <ChartContainer config={config} className="aspect-auto w-full" style={{ height: height + (names ? 28 : 0) }}>
      <RBarChart data={data} margin={{ top: unit ? 14 : 6, right: 4, bottom: 0, left: 0 }} title={t("Bar chart")}>
        <CartesianGrid vertical={false} />
        <XAxis dataKey="label" tickLine={false} axisLine={false} tick={axisTick} interval={0} />
        <YAxis domain={[0, max]} ticks={[0, max / 2, max]} tickFormatter={(v: number) => (max < 10 ? v.toFixed(1) : String(Math.round(v)))} tickLine={false} axisLine={false} tick={axisTick} width={28} />
        {unit && <text x="100%" y={10} dx={-2} textAnchor="end" fontSize={10} className="fill-muted-foreground">{unit}</text>}
        <ChartTooltip cursor={false} content={<ChartTooltipContent labelFormatter={(v) => (unit ? <>{v} <span className="font-normal text-muted-foreground">· {unit}</span></> : v)} />} />
        {names && <ChartLegend content={<ChartLegendContent />} />}
        {series.map((_, j) => <Bar key={j} dataKey={`s${String(j)}`} fill={`var(--color-s${String(j)})`} radius={2} isAnimationActive={false} />)}
      </RBarChart>
    </ChartContainer>
  );
}

/** An axis label sits at the start of the first tick, the end of the last, and the middle of the others. */
function tickAnchor(index: number, count: number): "start" | "end" | "middle" {
  if (index === 0) return "start";
  return index === count - 1 ? "end" : "middle";
}
/** One line of a line chart: its name and unit (the tooltip and the legend), its values (null is a gap), the axis it is read on and an optional guide value drawn on that axis. */
export interface ChartSeries { name: string; unit?: string; points: (number | null)[]; color?: string; axis?: "left" | "right"; threshold?: number; format?: (v: number) => string }
const seriesColors = ["var(--color-primary)", "var(--color-warn)", "var(--color-ok)"];
const noPoints: (number | null)[] = [];
/** The legend's text of a line: its name with its unit and, when the chart has two axes, the side it is read on. */
function legendText(l: ChartSeries, hasRight: boolean, t: Translate): string {
  const unit = l.unit ? ` (${l.unit})` : "";
  if (!hasRight) return `${l.name}${unit}`;
  return `${l.name}${unit} · ${t(l.axis === "right" ? "right axis" : "left axis")}`;
}
/** A value in the tooltip: formatted (`format`), with its unit; a gap reads "—". */
function tooltipValue(l: ChartSeries | undefined, value: unknown): string {
  if (typeof value !== "number") return "—";
  const text = l?.format ? l.format(value) : value.toLocaleString();
  return l?.unit ? `${text} ${l.unit}` : text;
}
/** The point a tooltip is on: the `i` of the hovered row, -1 when there is none. */
function hoveredIndex(payload: readonly { payload?: unknown }[]): number {
  const row = payload.at(0)?.payload;
  return typeof row === "object" && row !== null && "i" in row && typeof row.i === "number" ? row.i : -1;
}

/** A line chart; null points are gaps, never joined (a value between two gaps is a dot). `labels` spread evenly under
the chart, `ticks` draw gridlines with their values, `shadeGaps` shades the gaps. The tooltip names what each value is
(`name`, `unit`) and, with `times`, when it was measured (one heading per point). `series` draws several lines in one
chart, each on the left or the right axis with its own scale (IR689); `points` / `name` / `threshold` are the one-line form. */
export function LineChart({ points = noPoints, series, times, min, max, threshold, height = 120, color = "var(--color-primary)", labels, ticks, shadeGaps, name, unit, format }: Readonly<{ points?: (number | null)[]; series?: ChartSeries[]; times?: string[]; min?: number; max?: number; threshold?: number; height?: number; color?: string; labels?: string[]; ticks?: number[]; shadeGaps?: boolean; name?: string; unit?: string; format?: (v: number) => string }>) {
  const t = useT();
  const lines: ChartSeries[] = series ?? [{ name: name ?? t("Value"), points, color, ...(unit && { unit }), ...(format && { format }), ...(threshold !== undefined && { threshold }) }];
  const slots = Math.max(0, ...lines.map((l) => l.points.length));
  const last = Math.max(0, slots - 1);
  const valueAt = (l: ChartSeries, i: number) => l.points.at(i) ?? null;
  const data = Array.from({ length: slots }, (_, i) => ({ i, ...Object.fromEntries(lines.map((l, j) => [`v${String(j)}`, valueAt(l, i)])) }));
  const sideOf = (l: ChartSeries) => l.axis ?? "left";
  // each axis is scaled on its own lines (and their guides): the single-line form keeps its own scale (min / max / ticks)
  const scaleOf = (side: "left" | "right") => {
    const own = lines.filter((l) => sideOf(l) === side);
    if (own.length === 0) return null;
    if (!series) {
      const vals = points.filter((p): p is number => p !== null);
      return { domain: [min ?? Math.min(...vals, threshold ?? Infinity), max ?? Math.max(...vals, threshold ?? -Infinity)] as [number, number], ticks };
    }
    const guides = own.flatMap((l) => l.threshold ?? []);
    const range = axisRange(own.flatMap((l) => l.points), null);
    const lo = Math.min(range.min, ...guides), hi = Math.max(range.max, ...guides);
    return { domain: [lo, hi] as [number, number], ticks: axisTicks(lo, hi) };
  };
  const scales = { left: scaleOf("left"), right: scaleOf("right") };
  const isGap = (i: number) => lines.every((l) => valueAt(l, i) === null);
  const gaps: [number, number][] = [];
  for (let i = 0; i < slots; i++) {
    if (!isGap(i)) continue;
    const lastGap = gaps.at(-1);
    if (lastGap?.[1] === i - 1) lastGap[1] = i;
    else gaps.push([i, i]);
  }
  const isolated = (l: ChartSeries, i: number) => valueAt(l, i) !== null && (i === 0 || valueAt(l, i - 1) === null) && (i === last || valueAt(l, i + 1) === null);
  // the labels sit evenly from the first point to the last, the outer ones flush with the ends
  const at = (k: number) => (labels && labels.length > 1 ? (last * k) / (labels.length - 1) : 0);
  const isMulti = lines.length > 1;
  const mainAxis = scales.left ? "left" : "right";
  // the legend names each line with its unit and, with two axes, the side it is read on
  const config: ChartConfig = Object.fromEntries(lines.map((l, j) => [`v${String(j)}`, { label: legendText({ ...l, axis: sideOf(l) }, scales.right !== null, t), color: l.color ?? seriesColors.at(j % seriesColors.length) }]));
  return (
    <ChartContainer config={config} className="aspect-auto w-full" style={{ height: height + (isMulti ? 28 : 0) }}>
      <RLineChart data={data} margin={{ top: 8, right: 6, bottom: labels ? 0 : 4, left: 6 }} title={t("Line chart")}>
        {(ticks !== undefined || series !== undefined) && <CartesianGrid vertical={false} />}
        <XAxis type="number" dataKey="i" domain={[0, last]} hide={!labels} ticks={labels?.map((_, k) => at(k))} interval={0} tickLine={false} axisLine={false}
          tick={({ x, y, index }) => <text x={x} y={y} dy={8} fontSize={10} className="fill-muted-foreground" textAnchor={tickAnchor(index, labels?.length ?? 0)}>{labels?.at(index)}</text>} />
        {scales.left && <YAxis yAxisId="left" domain={scales.left.domain} hide={!scales.left.ticks} ticks={scales.left.ticks} mirror tickLine={false} axisLine={false} tick={axisTick} />}
        {scales.right && <YAxis yAxisId="right" orientation="right" domain={scales.right.domain} ticks={scales.right.ticks} mirror tickLine={false} axisLine={false} tick={axisTick} />}
        {shadeGaps && gaps.length > 0 && gaps.length < slots && gaps.map(([a, b]) => <ReferenceArea key={a} yAxisId={mainAxis} x1={Math.max(0, a - 0.5)} x2={Math.min(last, b + 0.5)} fill="#94a3b8" fillOpacity={0.14} />)}
        {lines.map((l, j) => l.threshold !== undefined && <ReferenceLine key={`g${String(j)}`} yAxisId={sideOf(l)} y={l.threshold} stroke="var(--color-crit)" strokeDasharray="4 3" />)}
        <ChartTooltip
          labelFormatter={(_, payload) => times?.at(hoveredIndex(payload)) ?? null}
          content={<ChartTooltipContent hideLabel={!times} formatter={(value, _name, item) => {
            const l = lines.at(Number(String(item.dataKey).slice(1)));
            return (
              <>
                <span className="h-2.5 w-2.5 shrink-0 rounded-[2px]" style={{ backgroundColor: item.color }} />
                <span className="flex flex-1 justify-between gap-3 leading-none">
                  <span className="text-muted-foreground">{l?.name}</span>
                  <span className="font-mono font-medium text-foreground tabular-nums">{tooltipValue(l, value)}</span>
                </span>
              </>
            );
          }} />} />
        {isMulti && <ChartLegend content={<ChartLegendContent />} />}
        {lines.map((l, j) => (
          <Line key={`l${String(j)}`} yAxisId={sideOf(l)} dataKey={`v${String(j)}`} name={l.name} type="linear" stroke={`var(--color-v${String(j)})`} strokeWidth={2} isAnimationActive={false}
            dot={({ index, cx: centerX, cy }) => (isolated(l, index) ? <circle key={index} cx={centerX} cy={cy} r={3} fill={`var(--color-v${String(j)})`} /> : null)} />
        ))}
      </RLineChart>
    </ChartContainer>
  );
}

const BAR_CLS = new Map<Tone, string>([["ok", "**:data-[slot=progress-indicator]:bg-ok"], ["warn", "**:data-[slot=progress-indicator]:bg-warn"], ["crit", "**:data-[slot=progress-indicator]:bg-crit"]]);
/** A progress bar (shadcn/ui's Progress); `label` names it for assistive technology (WCAG 4.1.2, IR359). */
export function UtilizationBar({ pct, tone = "primary", label }: Readonly<{ pct: number | null; tone?: Tone; label?: string }>) {
  return (
    <Progress value={pct} aria-label={label}
      className={cx("*:data-[slot=progress-track]:h-2", BAR_CLS.get(tone))} />
  );
}

