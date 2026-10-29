// Every icon of the four apps comes from one library, Lucide (lucide-react), through this component (DEC-74): one
// stroke, one size per text (1em), one baseline. Screens name what an icon means — `alerts`, `warning`, `edit` —
// and this map picks the drawing, so a meaning has the same icon everywhere. An icon next to text is decorative and
// hidden from assistive technology; an icon that stands alone (a close button, a search field) gets its name from the
// control it sits in, or from `label`.
import {
  AirVent, ArrowDown, ArrowLeft, ArrowLeftRight, ArrowRight, ArrowUp, Ban, Banknote, Bell, BellRing, Building2, CalendarClock,
  Camera, Check, ChevronDown, ChevronLeft, ChevronRight, ChevronUp, Circle, CircleAlert, CircleCheck, CircleDashed, CircleDot, CircleHelp, CirclePause, CircleSlash,
  CircleX, ClipboardCheck, ClipboardList, Clock, CloudSun, Cpu, Download, Ellipsis, ExternalLink, FileText, FlaskConical, Gauge, Globe, Handshake, History, House, Info, KeyRound,
  LayoutDashboard, Leaf, LoaderCircle, LocateFixed, Lock, LogOut, Mail, Menu, Mic, Minus, OctagonAlert, Pencil, PersonStanding, Play, Plug, Plus, Printer,
  QrCode, Receipt, RefreshCw, Repeat, ScrollText, Search, Send, Settings, Signature, Snowflake, Sparkles, Sprout, Square, SquareCheck,
  Star, Sun, Thermometer, Trash2, TriangleAlert, Undo2, Upload, UserCheck, UserRound, Users, Wifi, WifiOff, Wind, Workflow, Wrench, X, Zap,
  type LucideIcon,
} from "lucide-react";

const ICONS = {
  // navigation (one per sidebar item meaning)
  overview: LayoutDashboard,
  units: AirVent,
  customers: Building2,
  jobs: ClipboardList,
  history: History,
  schedule: CalendarClock,
  automations: Workflow,
  energy: Zap,
  airQuality: Wind,
  mrv: Leaf,
  offsets: Sprout,
  alerts: Bell,
  alertPolicies: BellRing,
  maintenance: Wrench,
  billing: Receipt,
  payouts: Banknote,
  contracts: FileText,
  devices: Cpu,
  restrictions: Ban,
  team: Users,
  users: Users,
  account: UserRound,
  access: KeyRound,
  audit: ScrollText,
  preferences: Settings,
  notifications: Bell,
  assistant: Sparkles,
  demo: FlaskConical,
  signOut: LogOut,
  menu: Menu,
  more: Ellipsis,
  // states
  info: Info,
  warning: TriangleAlert,
  critical: OctagonAlert,
  error: CircleX,
  success: CircleCheck,
  alert: CircleAlert,
  unknown: CircleHelp,
  pending: CircleDashed,
  check: Check,
  close: X,
  running: Play,
  stopped: Square,
  online: Wifi,
  offline: WifiOff,
  refresh: RefreshCw,
  locked: Lock,
  dot: Circle,
  live: CircleDot,
  unsupported: Ban,
  // job states (JobStatusBadge)
  requested: CircleDashed,
  proposed: ArrowLeftRight,
  offered: Send,
  accepted: Handshake,
  assigned: UserCheck,
  inProgress: LoaderCircle,
  submitted: ClipboardCheck,
  rework: Undo2,
  completed: CircleCheck,
  cancelled: CircleSlash,
  paused: CirclePause,
  checked: SquareCheck,
  unchecked: Square,
  // actions and things
  edit: Pencil,
  delete: Trash2,
  add: Plus,
  remove: Minus,
  search: Search,
  send: Send,
  print: Printer,
  download: Download,
  upload: Upload,
  external: ExternalLink,
  undo: Undo2,
  swap: ArrowLeftRight,
  star: Star,
  mic: Mic,
  camera: Camera,
  mail: Mail,
  signature: Signature,
  document: FileText,
  language: Globe,
  home: House,
  location: LocateFixed,
  presence: PersonStanding,
  routine: Repeat,
  weather: CloudSun,
  time: Clock,
  power: Plug,
  cooling: Snowflake,
  heat: Sun,
  temperature: Thermometer,
  meter: Gauge,
  qr: QrCode,
  chevronDown: ChevronDown,
  chevronUp: ChevronUp,
  chevronLeft: ChevronLeft,
  chevronRight: ChevronRight,
  arrowRight: ArrowRight,
  arrowLeft: ArrowLeft,
  arrowUp: ArrowUp,
  arrowDown: ArrowDown,
} satisfies Record<string, LucideIcon>;

export type IconName = keyof typeof ICONS;

const cls = (...a: (string | false | undefined)[]) => a.filter(Boolean).join(" ");

export function Icon({ name, className, label, filled }: { name: IconName; className?: string; label?: string; filled?: boolean }) {
  const Drawing = ICONS[name];
  return (
    <Drawing
      size="1em"
      strokeWidth={2}
      fill={filled ? "currentColor" : "none"}
      className={cls("inline-block shrink-0 align-[-0.125em]", className)}
      aria-hidden={label ? undefined : true}
      aria-label={label}
      role={label ? "img" : undefined}
      focusable="false"
    />
  );
}
