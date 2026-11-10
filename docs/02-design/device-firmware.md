# Device firmware requirements: ESP32 AC node (PoC)

Version: 0.4.0 / Created: 2026-11-05 / Status: Draft for review / Language: English (Japanese: [device-firmware.ja.md](device-firmware.ja.md))

This document is self-contained. It tells the firmware developer what the ESP32 node must do. No other document is needed.

## 1. Overview

A web platform shows and controls air conditioners (AC). Each AC has one ESP32 **node**. The node and the platform talk only through an **MQTT broker** (the broker address and credentials are given separately).

```
Web platform  <--- MQTT broker --->  ESP32 node  --IR-->  AC (Daikin)
                                         |
                                  BME680 / PZEM sensors
```

The node does three things:

1. Receives commands (power, temperature, mode, fan) and sends them to the AC as infrared (IR).
2. Reports sensor readings every 5 seconds.
3. Reports that it is alive (heartbeat) and what it last told the AC.

Two nodes exist today. The firmware is the same binary; only the settings differ.

| Node | `node_id` | Parts |
|---|---|---|
| 1 | `node_1` | IR LED (`ir`), BME680 (`bme680`). No power meter. |
| 2 | `node_2` | BME680, PZEM power meter (`pzem`). No IR. |

The platform learns which AC a node belongs to from its `node_id`: HQ registers each device in the platform's admin app with the **MQTT node ID** the node uses (exactly as set in the firmware, letters of either case, digits, `-` and `_`, one device per ID). A node whose ID nobody has registered is heard but not recorded, so a new node needs no platform release, only that registration.

Out of scope: TLS, per-node credentials, OTA update. The topics `…/sensors` and `…/command` and the plain-text commands already work in the current sketch and must keep working.

## 2. MQTT topics

All topics start with `smarthome/ac/<node_id>/`. Payloads are UTF-8 JSON unless stated.

| Topic (after the prefix) | Direction | QoS / retain | Content |
|---|---|---|---|
| `sensors` | node → platform | 0 / no (QoS 1 for replayed readings, §6) | Sensor readings (§5.2) |
| `command` | platform → node | 1 / no | Command (§3) |
| `command/result` | node → platform | 1 / no | Result of one command (§3.4) |
| `state` | node → platform | 1 / **retained** | Settings last sent to the AC (§4) |
| `status` | node → platform | 1 / **retained** | Heartbeat and last will (§5.1) |

The platform treats a node as **offline** when it hears nothing from it for 3 minutes, so something must arrive at least every 60 seconds.

## 3. Control: `…/command`

### 3.1 What we want

| ID | Want |
|---|---|
| DEV-01 | **Power ON / OFF.** |
| DEV-02 | **Set the temperature** to a given value, 16 to 30 °C in 1 °C steps (for example "26 °C"). |
| DEV-03 | **Set the mode** (`cool`, `dry`, `fan`, `auto`) and the **fan level** (`auto`, `low`, `mid`, `high`). |
| DEV-04 | One JSON command can carry any of these. A missing field keeps the current value. The old plain-text commands keep working. |
| DEV-05 | Every command gets exactly one **result** on `…/command/result`. An out-of-range value is `invalid` and sends no IR. |
| DEV-06 | Accept at most one IR command per second; extra ones are answered `invalid`. |

### 3.2 JSON command

```json
{"commandId":"7f3c…","power":true,"celsius":26,"mode":"cool","fanLevel":"auto"}
```

| Field | Type | Values | If missing |
|---|---|---|---|
| `commandId` | string | any text; echoed back in the result | the result has `null` |
| `power` | bool | `true` / `false` | keep current |
| `celsius` | integer | 16 to 30 | keep current |
| `mode` | string | `cool`, `dry`, `fan`, `auto` | keep current |
| `fanLevel` | string | `auto`, `low`, `mid`, `high` | keep current |

- Only temperature: `{"commandId":"…","celsius":26}` changes only the set temperature; power, mode and fan stay as they are.
- The node sends **one IR frame** built from the current settings with the new fields applied.
- `power:false` sends the OFF frame and keeps the other settings for the next `power:true`.
- Settings at the very first boot: on, `cool`, 24 °C, fan `auto`.
- A payload starting with `{` is JSON.

### 3.3 Plain-text command (existing)

`ON`, `OFF`, `COOL24` (cool, 24 °C). Ignore surrounding spaces and letter case. They are answered like JSON commands, with `commandId` `null`.

### 3.4 Result: `…/command/result`

Sent after the IR frame has been sent, or at once if none is sent.

```json
{"commandId":"7f3c…","command":"<received text>","result":"sent","at":"2026-11-05T12:00:01+07:00"}
```

| `result` | Meaning |
|---|---|
| `sent` | The IR frame went out. (IR is one-way: this does not prove the AC obeyed.) |
| `unsupported` | Unknown command, or this node has no IR (`node_2`). |
| `invalid` | Broken JSON, value out of range, or the rate limit (DEV-06). |

Unknown commands are also logged on the serial port.

## 4. State: `…/state` (retained)

| ID | Want |
|---|---|
| DEV-10 | After every sent command, publish the settings last sent to the AC. |
| DEV-11 | Keep the settings in flash and restore them at boot, so `ON` after a restart sends the last settings. At boot, publish `state` again if settings are stored. |

```json
{"power":true,"celsius":26,"mode":"cool","fanLevel":"auto"}
```

This is only the node's record of what it sent; it is never read back from the AC.

## 5. Heartbeat and sensors

### 5.1 Heartbeat: `…/status` (retained)

| ID | Want |
|---|---|
| DEV-20 | **Heartbeat** at connect and every 60 s. |
| DEV-21 | **Last will** `{"online":false}` on the same topic, so the platform learns of a lost node at once. |

```json
{"online":true,"fw":"0.4.0","ip":"10.100.1.50","rssi":-52,"uptime_s":123,"parts":{"ir":true,"bme680":true,"pzem":false}}
```

### 5.2 Sensors: `…/sensors`

| ID | Want |
|---|---|
| DEV-30 | Send readings every 5 s. |
| DEV-31 | A sensor that is absent or failed is **left out** (no key). Never send `0` for "not measured"; send `0` only when `0` was measured. |
| DEV-32 | Every message has `timestamp` from NTP: ISO-8601 with the offset, for example `+07:00`. This lets a late message be recorded at the right time. |
| DEV-33 | `node_1` sends no `voltage`, `current`, `power`, `energy` (no PZEM). |

```json
{"device_id":"node_1","timestamp":"2026-11-05T12:00:00+07:00","temp":25.1,"humidity":60.0,"pressure":1010.2,"gas":123456.0,"approx_altitude":10.5}
```

| Key | Unit | Decimals | Part |
|---|---|---|---|
| `temp` | °C | 1 | BME680 |
| `humidity` | % | 1 | BME680 |
| `pressure` | hPa | 1 | BME680 |
| `gas` | Ω | 1 | BME680 |
| `approx_altitude` | m | 1 | BME680 |
| `voltage` | V | 1 | PZEM |
| `power` | W | 1 | PZEM |
| `current` | A | 3 | PZEM |
| `energy` | kWh | 3 | PZEM |

## 6. Offline buffer (store and forward)

While the broker cannot be reached the node keeps its readings and sends them after it reconnects.

| ID | Want |
|---|---|
| DEV-40 | While disconnected, store readings in **flash** (not RAM), so they survive a reboot or power cut. One reading per minute is enough while offline. |
| DEV-41 | After reconnect, send the stored readings **oldest first**, at a limited rate (for example 10 per second), alongside the live readings. Live readings and commands are not held up. |
| DEV-42 | Send stored readings with QoS 1 and delete one only after the broker confirms it. Mark a replayed message with `"buffered":true`; its `timestamp` is the original measurement time. |
| DEV-43 | When the store is full, drop the **oldest** readings. Capacity: at least 48 hours. |
| DEV-44 | Store nothing before NTP has synced (no valid time). |
| DEV-45 | Buffer readings only. Never replay commands late. `status` and `state` are retained, so they need no buffer. |

## 7. Robustness and setup

| ID | Want |
|---|---|
| DEV-50 | Wi-Fi and MQTT reconnect by themselves **without blocking** the main loop (retry with back-off, 1 s up to 30 s). Sensors keep reading while the broker is down. Re-subscribe to `…/command` on every connect. |
| DEV-51 | `node_id` is a single setting. The `device_id` field, the topics, the MQTT client ID (for example `ac-<node_id>`; today both nodes use the same ID and kick each other off) and the log prefix all come from it. |
| DEV-52 | Wi-Fi, broker address and `node_id` are not hard-coded in the sketch (read them from flash or an untracked header; the current values may stay as defaults for the PoC). |
| DEV-53 | The MQTT packet size must fit the readings (512 bytes or more; PubSubClient's default is 256 and a longer message fails silently). Check the result of every `publish()`, log a failure, and send that reading to the offline buffer. |

## 8. Open points

| ID | Question |
|---|---|
| DEV-OPEN-01 | Which AC brands must be supported? The current sketch uses Daikin (`ir_Daikin.h`). The `mode` and `fanLevel` values must be checked for another brand. |
| DEV-OPEN-02 | Should `node_2` also send IR later? |
| DEV-OPEN-03 | Should pressure, gas and altitude be sent at all times, or only when asked? (The platform does not use them yet.) |
