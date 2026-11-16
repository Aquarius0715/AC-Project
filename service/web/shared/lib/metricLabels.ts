// The names of the alert metrics (English, also the dictionary keys), shared by the alert screens and the evidence
// wording (IR812), which cannot import the alert module (it imports the evidence one).
export const metricLabel: Record<string, string> = {
  temperature: "Temperature", humidity: "Humidity", co2: "CO₂ (ppm)", pm25: "PM2.5", power: "Power", vibration: "Vibration",
  refrigerant_pressure: "Refrigerant pressure", compressor_cycles: "Compressor cycles", airflow_drop: "Airflow drop", heartbeat_gap: "Heartbeat gap",
};
/** The metric's name where its value or the condition writes the unit (chart chips, "CO₂ ≥ 1200 ppm"): metricLabel
without the "(ppm)" that a metric picker shows (UI/UX loop R135). */
export const metricName: Record<string, string> = { ...metricLabel, co2: "CO₂" };
