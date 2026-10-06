export type UnitRow = { id: string; name: string; loc: string; obs: string; temp: number | null; hum: number | null; power: number | null; state: "running" | "stopped" | "unknown"; conn: "online" | "offline" };

export const units: UnitRow[] = [
  { id: "unit-online-rto", name: "Bedroom AC", loc: "Home A › 1F › Bedroom", obs: "09:12", temp: 28.0, hum: 60, power: 680, state: "running", conn: "online" },
  { id: "unit-bedroom-2", name: "Bedroom AC #2", loc: "Home A › 1F › Bedroom", obs: "09:12", temp: 27.0, hum: 58, power: 0, state: "stopped", conn: "online" },
  { id: "unit-living", name: "Living room AC", loc: "Home A › 1F › Living room", obs: "09:11", temp: 26.5, hum: 55, power: 510, state: "running", conn: "online" },
  { id: "unit-kitchen", name: "Kitchen AC", loc: "Home A › 1F › Kitchen", obs: "09:12", temp: 27.2, hum: 58, power: 545, state: "running", conn: "online" },
  { id: "unit-offline-rto", name: "Study AC", loc: "Home A › 2F › Study", obs: "last seen Sep 22 18:40", temp: null, hum: null, power: null, state: "unknown", conn: "offline" },
];

export const week = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];
