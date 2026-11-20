#if DEBUG
import ACKit
import Foundation

/// The technician's view of the UI tests' AC u1 (AC_UI_TEST=stub): its latest temperature, its indoor components, one
/// open alert, the readings of the last hours and its device with one event.
enum StubUnitData {
    static func at(_ secondsAgo: TimeInterval) -> String { Date().addingTimeInterval(-secondsAgo).ISO8601Format() }

    static func reading(_ value: Double, secondsAgo: TimeInterval) -> CoreMeasurement {
        CoreMeasurement(
            id: "me-\(Int(secondsAgo))",
            tenantId: "t1",
            version: 1,
            createdAt: at(secondsAgo),
            updatedAt: at(secondsAgo),
            boundaryId: nil,
            qualityReason: nil,
            rawUnit: nil,
            unitId: "u1",
            sensorId: "s1",
            metric: .temperature,
            value: value,
            unit: .c,
            observedAt: at(secondsAgo),
            receivedAt: at(secondsAgo),
            origin: .measured,
            quality: .valid,
            sequence: 1,
            eventId: "e1"
        )
    }

    static let components: [ComponentKey] = [.filter, .evaporatorCoil, .blowerMotor, .blowerFan, .drainPipe, .drainPan, .outlet, .louver]

    static var alert: CoreAlert {
        CoreAlert(
            id: "al1",
            tenantId: "t1",
            version: 1,
            createdAt: at(1800),
            updatedAt: at(1800),
            unitId: "u1",
            policyId: nil,
            type: .sensor,
            severity: .warning,
            status: .open,
            causeCode: .unknown,
            evidenceKind: .demoObservation,
            evidenceText: "Demo: high temperature observed (fictional)",
            observedAt: at(1800),
            evidenceIds: [],
            detectedAt: at(1800),
            acknowledgedAt: nil,
            resolvedAt: nil,
            resolutionReason: nil,
            previousAlertId: nil,
            deliveryFailures: [],
            rule: nil
        )
    }

    /// The day's 5-minute averages: every bucket of the last 6 hours, a gap of an hour in them.
    static var buckets: [SeriesBucket] {
        var buckets: [SeriesBucket] = []
        for ago in stride(from: 6 * 3600, to: 0, by: -300) where !(2 * 3600 < ago && ago < 3 * 3600) {
            let value = 24 + Double(ago % 3600) / 1800
            buckets.append(SeriesBucket(start: at(TimeInterval(ago)), value: value, count: 60))
        }
        return buckets
    }

    static var series: AirSeries {
        AirSeries(
            items: [],
            nextCursor: nil,
            total: Double(buckets.count * 60),
            snapshotVersion: 1,
            allergenObservation: nil,
            buckets: buckets
        )
    }
}
#endif
