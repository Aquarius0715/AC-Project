"use client";

import Link from "next/link";
import { Children, createContext, isValidElement, useCallback, useContext, useRef, useState } from "react";
import { useT } from "@ac/web/components/I18n";
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
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@ac/web/components/ui/table";
import { Tabs as UITabs, TabsList, TabsTrigger } from "@ac/web/components/ui/tabs";
import { Toaster } from "@ac/web/components/ui/sonner";
import { Textarea as UITextarea } from "@ac/web/components/ui/textarea";
import { ToggleGroup, ToggleGroupItem } from "@ac/web/components/ui/toggle-group";
import { cn } from "@ac/web/lib/utils";
import { tableFeatures, useTable, type ColumnDef, type RowData } from "@tanstack/react-table";
import { Bar, BarChart as RBarChart, CartesianGrid, Line, LineChart as RLineChart, ReferenceArea, ReferenceLine, XAxis, YAxis } from "recharts";
import { toast } from "sonner";

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
export function Field({ label, hint, error, children, className, labelHidden }: { label: string; hint?: React.ReactNode; error?: React.ReactNode; children: React.ReactNode; className?: string; labelHidden?: boolean }) {
  // Only a single form control sits inside a <label>. Anything else — choice buttons, a set of checkboxes or buttons, a
  // read-only box — is a labelled group: inside a <label> a button takes the label as its name (IR230, IR238).
  // `labelHidden` keeps the label for assistive technology only, for a group that shows labels of its own.
  const control = isValidElement(children) && (children.type === Input || children.type === Select || children.type === Textarea || ["input", "select", "textarea"].includes(children.type as string));
  const group = !control;
  const Tag = group ? "div" : "label";
  return (
    <Tag role={group ? "group" : undefined} aria-label={group ? label : undefined} className={cx("flex min-w-0 flex-col gap-1 text-xs text-ink", className)}>
      <span aria-hidden={group || undefined} className={cx("font-semibold", labelHidden && "sr-only")}>{label}</span>
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
/** A button with no look of its own, like the browser's button under Tailwind's reset: the text, size and layout come from
 * its parent and its classes. A screen uses it for a selectable card, a chip or a text action, so the shadcn Button's
 * height, padding, radius and weight are reset here rather than in each screen. */
export const Plain = ({ className, ...p }: React.ComponentProps<typeof Button>) => (
  <Button {...p} variant="ghost" className={cn("inline-block h-auto gap-0 rounded-none p-0 font-[inherit] whitespace-normal justify-start leading-[inherit] [font-size:inherit] [letter-spacing:inherit] active:translate-y-0 disabled:opacity-100 [&_svg:not([class*='size-'])]:size-auto", className)} />
);
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

/** A select for long lists (DD-A03 search-selects): the shadcn/ui Combobox. Typing narrows the options, the first
 * `limit` matches are listed, the arrow keys and Enter choose one and Escape closes; closed, it shows the chosen
 * option. `label` names the input and its list. */
export function SearchSelect({ value, onChange, options, label, placeholder, limit = 20, className }: {
  value: string; onChange: (id: string) => void; options: { id: string; label: string }[]; label: string; placeholder?: string; limit?: number; className?: string;
}) {
  const t = useT();
  const [query, setQuery] = useState("");
  const current = options.find((o) => o.id === value) ?? null;
  const needle = query.trim().toLowerCase();
  // the chosen option's own label in the input lists everything, as an empty input does
  const matches = !needle || needle === current?.label.toLowerCase() ? options : options.filter((o) => o.label.toLowerCase().includes(needle));
  return (
    <Combobox items={options} filteredItems={matches} limit={limit} value={current} openOnInputClick
      itemToStringLabel={(o: { id: string; label: string }) => o.label} isItemEqualToValue={(a: { id: string }, b: { id: string }) => a.id === b.id}
      onInputValueChange={(q) => setQuery(q)} onValueChange={(o) => { if (o && o.id !== value) onChange(o.id); }}>
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
/** The toasts: shadcn/ui's Sonner. Sonner announces through one polite region, so an error carries its text in an alert
 * (announced at once) and every other result in a status (UX-06, IR362). */
export function ToastProvider({ children }: { children: React.ReactNode }) {
  const push = useCallback((msg: string, tone: Tone = "ok") => {
    const body = <span role={tone === "crit" ? "alert" : "status"}>{msg}</span>;
    if (tone === "crit") toast.error(body);
    else if (tone === "warn") toast.warning(body);
    else if (tone === "ok") toast.success(body);
    else toast.info(body);
  }, []);
  return (
    <ToastCtx.Provider value={push}>
      {children}
      <Toaster position="bottom-center" />
    </ToastCtx.Provider>
  );
}

/* ───────────── Data display ───────────── */
export type Col<T> = { key: string; label: string; render: (r: T) => React.ReactNode; className?: string; hideBelow?: "sm" | "md" };
// TanStack Table builds the rows and cells (shadcn/ui's data table, DEC-75); a column carries its classes as meta
const tableFx = tableFeatures({ columnMeta: {} as { className?: string; hideBelow?: "sm" | "md" } });
const hideCls = (m?: { hideBelow?: "sm" | "md" }) => (m?.hideBelow === "sm" ? "max-sm:hidden" : m?.hideBelow === "md" ? "max-md:hidden" : undefined);
/** Responsive table: scrolls horizontally if needed; optional columns hide on narrow containers. A row that opens a
 * record (`onRowClick`) is focusable and opens with Enter or Space as well as a click (keyboard access, IR294). */
export function DataTable<T extends RowData>({ cols, rows, rowKey, onRowClick, selectedKey }: { cols: Col<T>[]; rows: T[]; rowKey: (r: T) => string; onRowClick?: (r: T) => void; selectedKey?: string }) {
  const t = useT();
  const columns = cols.map((c): ColumnDef<typeof tableFx, T> => ({ id: c.key, header: c.label, cell: ({ row }) => c.render(row.original), meta: { className: c.className, hideBelow: c.hideBelow } }));
  const table = useTable({ features: tableFx, data: rows, columns, getRowId: (r) => rowKey(r) });
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
            const selected = selectedKey === row.id;
            return (
              <TableRow key={row.id} onClick={onRowClick && (() => onRowClick(r))} tabIndex={onRowClick ? 0 : undefined} aria-current={selected ? "true" : undefined} data-state={selected ? "selected" : undefined}
                onKeyDown={onRowClick && ((e) => { if (e.target === e.currentTarget && (e.key === "Enter" || e.key === " ")) { e.preventDefault(); onRowClick(r); } })}
                className={cx("data-[state=selected]:bg-primary-soft/60", onRowClick && "cursor-pointer outline-none focus-visible:bg-muted/60 focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-primary")}>
                {row.getAllCells().map((cell) => (
                  <TableCell key={cell.id} className={cx("px-3 py-2.5 whitespace-normal", hideCls(cell.column.columnDef.meta), cell.column.columnDef.meta?.className)}>
                    <table.FlexRender cell={cell} />
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
export function ListRow({ selected, onClick, href, children, className }: { selected?: boolean; onClick?: () => void; href?: string; children: React.ReactNode; className?: string }) {
  const cls = cx("flex w-full min-w-0 items-center gap-3 rounded-xl border px-3 py-2.5 text-left transition-colors", selected ? "border-primary bg-primary-soft/60" : "border-line bg-surface hover:bg-surface2/70", className);
  if (href) return <Link href={href} className={cls}>{children}</Link>;
  return <button type="button" onClick={onClick} aria-pressed={selected} className={cls}>{children}</button>;
}

/* ───────────── Charts: shadcn/ui's chart on Recharts (DEC-75); full width, `height` pixels tall ───────────── */
const axisTick = { fontSize: 10 };
/** Grouped bars, one group per label. `names` name the series in the tooltip and the legend under the chart. */
export function BarChart({ labels, series, names, unit, height = 160, colors = ["#c9d8ee", "#005bea"] }: { labels: string[]; series: number[][]; names?: string[]; unit?: string; height?: number; colors?: string[] }) {
  const t = useT();
  const max = Math.max(1, ...series.flat()) * 1.1;
  const data = labels.map((label, i) => ({ label, ...Object.fromEntries(series.map((s, j) => [`s${j}`, s[i]])) }));
  const config: ChartConfig = Object.fromEntries(series.map((_, j) => [`s${j}`, { label: names?.[j] ?? t("Value"), color: colors[j % colors.length] }]));
  return (
    <ChartContainer config={config} className="aspect-auto w-full" style={{ height: height + (names ? 28 : 0) }}>
      <RBarChart data={data} margin={{ top: unit ? 14 : 6, right: 4, bottom: 0, left: 0 }} title={t("Bar chart")}>
        <CartesianGrid vertical={false} />
        <XAxis dataKey="label" tickLine={false} axisLine={false} tick={axisTick} interval={0} />
        <YAxis domain={[0, max]} ticks={[0, max / 2, max]} tickFormatter={(v: number) => (max < 10 ? v.toFixed(1) : String(Math.round(v)))} tickLine={false} axisLine={false} tick={axisTick} width={28} />
        {unit && <text x="100%" y={10} dx={-2} textAnchor="end" fontSize={10} className="fill-muted-foreground">{unit}</text>}
        <ChartTooltip cursor={false} content={<ChartTooltipContent labelFormatter={(v) => (unit ? <>{v} <span className="font-normal text-muted-foreground">· {unit}</span></> : v)} />} />
        {names && <ChartLegend content={<ChartLegendContent />} />}
        {series.map((_, j) => <Bar key={j} dataKey={`s${j}`} fill={`var(--color-s${j})`} radius={2} isAnimationActive={false} />)}
      </RBarChart>
    </ChartContainer>
  );
}

/** A line chart; null points are gaps, never joined (a value between two gaps is a dot). `labels` spread evenly under
 * the chart, `ticks` draw gridlines with their values, `shadeGaps` shades the gaps; `name` names the value in the tooltip. */
export function LineChart({ points, min, max, threshold, height = 120, color = "var(--color-primary)", labels, ticks, shadeGaps, name }: { points: (number | null)[]; min?: number; max?: number; threshold?: number; height?: number; color?: string; labels?: string[]; ticks?: number[]; shadeGaps?: boolean; name?: string }) {
  const t = useT();
  const vals = points.filter((p): p is number => p !== null);
  const lo = min ?? Math.min(...vals, threshold ?? Infinity);
  const hi = max ?? Math.max(...vals, threshold ?? -Infinity);
  const last = Math.max(0, points.length - 1);
  const data = points.map((v, i) => ({ i, v }));
  const gaps: [number, number][] = [];
  points.forEach((p, i) => {
    if (p !== null) return;
    if (gaps.length && gaps[gaps.length - 1][1] === i - 1) gaps[gaps.length - 1][1] = i;
    else gaps.push([i, i]);
  });
  const isolated = (i: number) => points[i] !== null && (i === 0 || points[i - 1] === null) && (i === last || points[i + 1] === null);
  // the labels sit evenly from the first point to the last, the outer ones flush with the ends
  const at = (k: number) => (labels && labels.length > 1 ? (last * k) / (labels.length - 1) : 0);
  return (
    <ChartContainer config={{ v: { label: name ?? t("Value"), color } }} className="aspect-auto w-full" style={{ height }}>
      <RLineChart data={data} margin={{ top: 8, right: 6, bottom: labels ? 0 : 4, left: 6 }} title={t("Line chart")}>
        {ticks && <CartesianGrid vertical={false} />}
        <XAxis type="number" dataKey="i" domain={[0, last]} hide={!labels} ticks={labels?.map((_, k) => at(k))} interval={0} tickLine={false} axisLine={false}
          tick={({ x, y, index }) => <text x={x} y={y} dy={8} fontSize={10} className="fill-muted-foreground" textAnchor={index === 0 ? "start" : index === (labels?.length ?? 0) - 1 ? "end" : "middle"}>{labels?.[index]}</text>} />
        <YAxis domain={[lo, hi]} hide={!ticks} ticks={ticks} mirror tickLine={false} axisLine={false} tick={axisTick} />
        {shadeGaps && vals.length > 0 && gaps.map(([a, b]) => <ReferenceArea key={a} x1={Math.max(0, a - 0.5)} x2={Math.min(last, b + 0.5)} fill="#94a3b8" fillOpacity={0.14} />)}
        {threshold !== undefined && <ReferenceLine y={threshold} stroke="var(--color-crit)" strokeDasharray="4 3" />}
        <ChartTooltip content={<ChartTooltipContent hideLabel />} />
        <Line dataKey="v" type="linear" stroke="var(--color-v)" strokeWidth={2} isAnimationActive={false}
          dot={({ index, cx, cy }) => (isolated(index) ? <circle key={index} cx={cx} cy={cy} r={3} fill="var(--color-v)" /> : null)} />
      </RLineChart>
    </ChartContainer>
  );
}

/** A progress bar (shadcn/ui's Progress); `label` names it for assistive technology (WCAG 4.1.2, IR359). */
export function UtilBar({ pct, tone = "primary", label }: { pct: number | null; tone?: Tone; label?: string }) {
  return (
    <Progress value={pct} aria-label={label}
      className={cx("*:data-[slot=progress-track]:h-2", tone === "ok" ? "**:data-[slot=progress-indicator]:bg-ok" : tone === "warn" ? "**:data-[slot=progress-indicator]:bg-warn" : tone === "crit" ? "**:data-[slot=progress-indicator]:bg-crit" : undefined)} />
  );
}

/* Form-in-modal helper: required-field validation */
export function useRequired<T extends Record<string, string>>(init: T) {
  const [v, setV] = useState<T>(init);
  const [submitted, setSubmitted] = useState(false);
  const err = (k: keyof T, msg: string, min = 1) => (submitted && v[k].trim().length < min ? msg : undefined);
  return { v, set: (k: keyof T, val: string) => setV((s) => ({ ...s, [k]: val })), submitted, submit: () => setSubmitted(true), reset: () => { setV(init); setSubmitted(false); }, err };
}
