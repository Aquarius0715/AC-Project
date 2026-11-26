import Foundation

/// An alert's evidence text in the display language (shared/lib/evidence.ts, IR480): the texts the API's sources write
/// (the seed, the filter-care reminders, the policy rules, the tamper and restriction-bypass alerts) in words; any other
/// text stays as stored. Names, serials and numbers inside a text are kept as they are.
public enum Evidence {
    public static func text(_ stored: String) -> String {
        if let fixed = fixed(stored) { return fixed }
        if let match = stored.wholeMatch(of: /Cleaning due \((\d+) h of run time since the last cleaning\)\. Not a fault\./) {
            return tr("Cleaning due ({hours} h of run time since the last cleaning). Not a fault.", ["hours": String(match.1)])
        }
        if let match = stored.wholeMatch(of: /Cleaning due \((\d+) h of run time, no cleaning recorded\)\. Not a fault\./) {
            return tr("Cleaning due ({hours} h of run time, no cleaning recorded). Not a fault.", ["hours": String(match.1)])
        }
        if let match = stored.wholeMatch(of: /Cleaning due \((\d+) days since the last cleaning, run time unknown\)\. Not a fault\./) {
            return tr("Cleaning due ({days} days since the last cleaning, run time unknown). Not a fault.", ["days": String(match.1)])
        }
        if let match = stored.wholeMatch(of: /Device (\S+) reported its cover opened while powered \(tamper_signal\)/) {
            return tr("Device {serial} reported its cover opened while powered", ["serial": String(match.1)])
        }
        return bypass(stored) ?? demoEvent(stored) ?? stored
    }

    private static func fixed(_ stored: String) -> String? {
        switch stored {
        case "Cleaning due. Not a fault.": tr("Cleaning due. Not a fault.")
        case "Device still reports an ended restriction; reconcile it from HQ":
            tr("Device still reports an ended restriction; reconcile it from HQ")
        case "Demo insulation": tr("Demo insulation")
        case "Demo observation": tr("Demo observation")
        case "Demo: cooling load rose while the outdoor temperature was stable.":
            tr("Demo: cooling load rose while the outdoor temperature was stable.")
        case "Demo: cooling load rose; no cause evidence is available (fictional).":
            tr("Demo: cooling load rose; no cause evidence is available (fictional).")
        case "Demo: high temperature observed (fictional)": tr("Demo: high temperature observed (fictional)")
        case "Demo: inspection recorded worn insulation around the indoor unit piping (fictional).":
            tr("Demo: inspection recorded worn insulation around the indoor unit piping (fictional).")
        case "Demo: synthetic allergen observation without unit (fictional).":
            tr("Demo: synthetic allergen observation without unit (fictional).")
        case "Demo: synthetic allergen observation; not a health assessment.":
            tr("Demo: synthetic allergen observation; not a health assessment.")
        case "Demo: temperature at or above 30°C for 60 seconds (fictional)":
            tr("Demo: temperature at or above 30°C for 60 seconds (fictional)")
        default: nil
        }
    }

    /// A restriction-bypass alert (DEC-110, IR847): the device, the restriction kind in words.
    private static func bypass(_ stored: String) -> String? {
        let restriction: (Substring) -> String = { kind in
            switch kind {
            case "power_off": tr("Power off")
            case "temperature_limit": tr("Temperature limit")
            default: String(kind)
            }
        }
        if let m = stored.wholeMatch(
            of: /Device (\S+) stopped reporting while the unit is under a (\S+) restriction: the restriction may no longer hold/
        ) {
            return tr(
                "Device {serial} stopped reporting while the unit is under a restriction ({restriction}): the restriction may no longer hold",
                ["serial": String(m.1), "restriction": restriction(m.2)]
            )
        }
        if let m = stored.wholeMatch(
            of: /Device (\S+) lost power while the unit is under a (\S+) restriction: the restriction may no longer hold/
        ) {
            return tr(
                "Device {serial} lost power while the unit is under a restriction ({restriction}): the restriction may no longer hold",
                ["serial": String(m.1), "restriction": restriction(m.2)]
            )
        }
        if let m = stored.wholeMatch(
            of: /Device (\S+) reported its cover opened while the unit is under a (\S+) restriction: the restriction may no longer hold/
        ) {
            return tr(
                "Device {serial} reported its cover opened while the unit is under a restriction ({restriction}): the restriction may no longer hold",
                ["serial": String(m.1), "restriction": restriction(m.2)]
            )
        }
        return nil
    }

    /// A demo event of a policy rule (IR812): the metric and the comparison in words.
    private static func demoEvent(_ stored: String) -> String? {
        guard let m = stored.wholeMatch(of: /Demo event: (\S+) (\S+) threshold/) else { return nil }
        let operators = ["gt": tr("above"), "gte": tr("at or above"), "lt": tr("below"), "lte": tr("at or below")]
        return tr(
            "Demo event: {metric} {operator} its alert threshold",
            ["metric": metric(m.1), "operator": operators[String(m.2)] ?? String(m.2)]
        )
    }

    /// A metric's name (metricLabels.ts metricName).
    public static func metric(_ code: Substring) -> String {
        switch code {
        case "temperature": tr("Temperature")
        case "humidity": tr("Humidity")
        case "co2": tr("CO₂")
        case "pm25": tr("PM2.5")
        case "power": tr("Power")
        case "vibration": tr("Vibration")
        case "refrigerant_pressure": tr("Refrigerant pressure")
        case "compressor_cycles": tr("Compressor cycles")
        case "airflow_drop": tr("Airflow drop")
        case "heartbeat_gap": tr("Heartbeat gap")
        default: String(code)
        }
    }
}
