# Device firmware requirements: ESP32 AC node (PoC)

Version: 0.5.0 / Created: 2026-11-05 / Updated: 2026-11-10 (follows firmware 0.4.0, `service/device/main.cpp`) / Status: Draft for review / Language: English (Japanese: [device-firmware.ja.md](device-firmware.ja.md))

This document is self-contained. It tells the firmware developer what the ESP32 node must do. No other document is needed. Since 0.5.0 the formats are those of firmware 0.4.0 as built; where the firmware does less than an earlier draft asked, the earlier want is listed under §8.

## 1. Overview

A web platform shows and controls air conditioners (AC). Each AC has one ESP32 **node**. The node and the platform talk only through an **MQTT broker** (the broker address and credentials are given separately).

```
Web platform  <--- MQTT broker --->  ESP32 node  --IR-->  AC (Daikin)
                                         |
                              BME680 / PM2.5 / PZEM sensors
```

The node does three things:

1. Receives commands (power, temperature, mode, fan) and sends them to the AC as infrared (IR).
2. Reports sensor readings every 5 seconds.
3. Reports that it is alive (heartbeat) and what it last told the AC.

Two nodes exist today. The firmware is the same binary; only `NODE_ID` (in `config.h`) differs. Each node reports the parts it found in its heartbeat (§5.1).

| Node | `node_id` | Parts |
|---|---|---|
| 1 | `node_1` | IR LED (`ir`), BME680 (`bme680`). Its IR LED sets the AC. |
| 2 | `node_2` | BME680, PM2.5 sensor (`pm25`, DFRobot air quality sensor on I²C 0x19). No IR. A PZEM power meter (`pzem`) when one is wired and powered. |

The platform learns which AC a node belongs to from its `node_id`: HQ registers each device in the platform's admin app with the **MQTT node ID** the node uses (exactly as set in the firmware, letters of either case, digits, `-` and `_`, one device per ID). A node whose ID nobody has registered is heard but not recorded, so a new node needs no platform release, only that registration.

Out of scope: TLS, per-node credentials, OTA update. The topics `…/sensors` and `…/command` and the plain-text commands of the first sketch keep working.

## 2. MQTT topics

All topics start with `smarthome/ac/<node_id>/`. Payloads are UTF-8 JSON unless stated. PubSubClient publishes at QoS 0 only, so everything the node sends is QoS 0.

| Topic (after the prefix) | Direction | QoS / retain | Content |
|---|---|---|---|
| `sensors` | node → platform | 0 / no | Sensor readings (§5.2), and readings kept while offline (§6) |
| `command` | platform → node | 1 / no (the node subscribes at QoS 0) | Command (§3) |
| `command/result` | node → platform | 0 / no | Result of one command (§3.4) |
| `state` | node → platform | 0 / **retained** | Settings last sent to the AC (§4) |
| `status` | node → platform | 0 / **retained** | Heartbeat and last will (§5.1) |

The platform treats a node as **offline** when it hears nothing from it for 3 minutes, or at once when the broker sends the node's last will (§5.1). Readings every 5 seconds and the heartbeat every 60 seconds keep it online.

## 3. Control: `…/command`

### 3.1 What we want

| ID | Want |
|---|---|
| DEV-01 | **Power ON / OFF.** |
| DEV-02 | **Set the temperature** to a given value, 16 to 30 °C in 1 °C steps (for example "26 °C"). |
| DEV-03 | **Set the mode** (`cool`, `dry`, `fan`, `auto`) and the **fan level** (`auto`, `low`, `mid`, `high`). |
| DEV-04 | One JSON command can carry any of these. A missing field keeps the current value. The old plain-text commands keep working. |
| DEV-05 | Every command gets exactly one **result** on `…/command/result`. An out-of-range value is `invalid` and sends no IR. |
| DEV-06 | Accept at most one IR command per second; one that comes within a second of the last sent one is answered `invalid`. |

### 3.2 JSON command

```json
{"commandId":"7f3c…","power":"ON","temp":26,"mode":"cool","fan":"auto"}
```

| Field | Type | Values | If missing |
|---|---|---|---|
| `commandId` | string | any text; echoed back in the result | the result has `null` |
| `power` | string | `ON` / `OFF` | keep current |
| `temp` | integer | 16 to 30 | keep current |
| `mode` | string | `cool`, `dry`, `fan`, `auto` | keep current |
| `fan` | string | `auto`, `low`, `mid`, `high` | keep current |

- Only temperature: `{"commandId":"…","temp":26}` changes only the set temperature; power, mode and fan stay as they are.
- The node sends **one IR frame** built from the current settings with the new fields applied. While the power is `OFF` it sends the OFF frame and keeps the new settings for the next `ON`.
- Settings at the very first boot: off, `cool`, 24 °C, fan `auto`.
- A payload that parses as a JSON object is a JSON command; anything else is a plain-text command (§3.3).
- The platform sends one setting per command (power, `temp`, `mode` or `fan`) with its command ID, and only the platform's values: modes `cool`, `dry`, `fan`, fan levels `low`, `mid`, `high`. Until a node has sent a heartbeat with its firmware version (§5.1), the platform sends it the plain-text commands, power only.

### 3.3 Plain-text command (existing)

`ON`, `OFF`, `COOL24` (on, cool, 24 °C, fan `auto`). Surrounding spaces and letter case are ignored. They are answered like JSON commands, with `commandId` `null`.

### 3.4 Result: `…/command/result`

Sent after the IR frame and the new `state` have been sent, or at once if no IR is sent.

```json
{"commandId":"7f3c…","command":"<received text>","result":"sent","at":"2026-11-05T12:00:01+07:00"}
```

| `result` | Meaning |
|---|---|
| `sent` | The IR frame went out. (IR is one-way: this does not prove the AC obeyed.) |
| `unsupported` | A plain-text command it does not know, or a payload that is no JSON object. |
| `invalid` | A value out of range or not one of the words above, or the rate limit (DEV-06). |

`at` is the node's time (§5.2), `N/A` before NTP has synced. Unknown commands are also logged on the serial port. The platform takes `sent` as the command's acknowledgement and `invalid` / `unsupported` as its rejection; a command that gets no result ends as expired.

## 4. State: `…/state` (retained)

| ID | Want |
|---|---|
| DEV-10 | After every sent command, publish the settings last sent to the AC. |
| DEV-11 | Keep the settings in flash (`Preferences`, namespace `ac_state`) and restore them at boot, so `ON` after a restart sends the last settings. Publish `state` again on every connect to the broker. |

```json
{"power":"ON","temp":26,"mode":"cool","fan":"auto"}
```

The values are the words of §3.2. This is only the node's record of what it sent; it is never read back from the AC. The platform shows these settings on the unit; `auto` mode and `auto` fan have no platform value and show as unknown.

## 5. Heartbeat and sensors

### 5.1 Heartbeat: `…/status` (retained)

| ID | Want |
|---|---|
| DEV-20 | **Heartbeat** at connect and every 60 s. |
| DEV-21 | **Last will** `{"online":false}` (retained) on the same topic, so the platform learns of a lost node at once. |

```json
{"online":true,"fw":"0.4.0","ip":"192.0.2.50","rssi":-52,"uptime_s":123,"parts":{"ir":true,"bme680":true,"pm25":false,"pzem":false}}
```

`parts.ir` is true on every node but `node_2`; `bme680` and `pm25` are true when the sensor answered at boot; `pzem` is true when the meter reads a voltage.

### 5.2 Sensors: `…/sensors`

| ID | Want |
|---|---|
| DEV-30 | Send readings every 5 s. |
| DEV-31 | A sensor that is absent or failed is **left out** (no key). Never send `0` for "not measured"; send `0` only when `0` was measured. |
| DEV-32 | Every message has `timestamp` from NTP: ISO-8601 with the offset, `YYYY-MM-DDTHH:MM:SS+07:00` (`N/A` before NTP has synced). This lets a late message be recorded at the right time. |
| DEV-33 | The PZEM fields (`voltage`, `current`, `power`, `energy`) are sent only while the meter reads a voltage above 0. |

```json
{"node_id":"node_2","timestamp":"2026-11-10T17:35:26+07:00","temp":25.6,"humidity":43.8,"pressure":1008.0,"gas":423837,"approx_altitude":43.9,"pm25":55}
```

| Key | Unit | Decimals | Part |
|---|---|---|---|
| `node_id` | — | — | the node's `NODE_ID` |
| `temp` | °C | 1 | BME680 |
| `humidity` | % | 1 | BME680 |
| `pressure` | hPa | 1 | BME680 |
| `gas` | Ω | 0 | BME680 |
| `approx_altitude` | m | 1 | BME680 |
| `pm25` | µg/m³ | 0 | PM2.5 sensor |
| `voltage` | V | 1 | PZEM |
| `power` | W | 1 | PZEM |
| `current` | A | 3 | PZEM |
| `energy` | kWh | 3 | PZEM |

The platform records `temp`, `humidity`, `pm25` and `power`. The first sketch's messages (`device_id`, a timestamp without an offset, `0` for what a node does not measure) are still read: there a temperature, humidity or voltage of `0` counts as not measured.

## 6. Offline buffer (store and forward)

While the broker cannot be reached the node keeps its readings and sends them after it reconnects.

| ID | As built (firmware 0.4.0) |
|---|---|
| DEV-40 | While disconnected, and when a `publish()` fails, the reading goes to a **RAM** buffer of 50 messages (about 4 minutes at one every 5 s). It is lost on a reboot or power cut. |
| DEV-41 | After reconnect, right after the heartbeat and `state`, the node sends the buffered readings **oldest first**, 50 ms apart, before the live readings. It stops at the first failed `publish()` and keeps the rest. |
| DEV-42 | A replayed message is the original one: QoS 0, no extra field; its `timestamp` is the original measurement time. The platform records a reading whose `timestamp` is more than a minute older than its arrival at the minute of that `timestamp`. |
| DEV-43 | When the buffer is full, **new** readings are dropped (the oldest 50 are kept). |
| DEV-44 | A reading taken before NTP has synced has `timestamp` `N/A`; the platform records it at its arrival. |
| DEV-45 | Readings only are buffered. Commands are never replayed late. `status` and `state` are retained, so they need no buffer. |

## 7. Robustness and setup

| ID | Want |
|---|---|
| DEV-50 | Wi-Fi and MQTT reconnect by themselves **without blocking** the main loop (retry with back-off, 1 s up to 30 s). Sensors keep reading while the broker is down. Re-subscribe to `…/command` on every connect. |
| DEV-51 | `node_id` is a single setting. The `node_id` field, the topics, the MQTT client ID (`ac-<node_id>`) and the log prefix all come from it. |
| DEV-52 | Wi-Fi, broker address and `node_id` are not hard-coded in the sketch (they are in `config.h`; the current values may stay as defaults for the PoC). |
| DEV-53 | The MQTT packet size fits the readings (`setBufferSize(1024)`; PubSubClient's default is 256 and a longer message fails silently). A failed `publish()` of a reading sends it to the offline buffer. |

## 8. Open points

| ID | Question |
|---|---|
| DEV-OPEN-01 | Which AC brands must be supported? The current sketch uses Daikin (`ir_Daikin.h`). The `mode` and `fan` values must be checked for another brand. |
| DEV-OPEN-02 | Should `node_2` also send IR later? Today it has no IR LED but still answers `sent` (the check that would answer `unsupported` is commented out); the platform sends it no commands. |
| DEV-OPEN-03 | Should pressure, gas and altitude be sent at all times, or only when asked? (The platform does not use them yet.) |
| DEV-OPEN-04 | The earlier draft asked for a buffer in **flash** that survives a reboot, holds at least 48 hours at one reading a minute and drops the oldest when full, replayed at a limited rate beside the live readings with QoS 1 and `"buffered":true`. Is that still wanted? |
