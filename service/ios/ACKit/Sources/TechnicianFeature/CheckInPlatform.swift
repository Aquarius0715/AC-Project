#if os(iOS)
import ACKit
import CoreLocation
import SwiftUI
import VisionKit

/// The phone's position for a check-in (Core Location): the "while using" permission, then one fresh fix. The UI tests
/// and the live check drive it; the model's tests use a stand-in (PositionSource).
@MainActor
public final class CoreLocationPosition: NSObject, PositionSource, CLLocationManagerDelegate {
    private let manager = CLLocationManager()
    private var permissionWaiters: [CheckedContinuation<CLAuthorizationStatus, Never>] = []
    private var fixWaiters: [CheckedContinuation<PositionAnswer, Never>] = []

    override public init() {
        super.init()
        manager.delegate = self
        manager.desiredAccuracy = kCLLocationAccuracyBest
    }

    public func locate() async -> PositionAnswer {
        var status = manager.authorizationStatus
        if status == .notDetermined {
            status = await withCheckedContinuation { waiter in
                permissionWaiters.append(waiter)
                manager.requestWhenInUseAuthorization()
            }
        }
        guard status == .authorizedWhenInUse || status == .authorizedAlways else { return .refused }
        return await withCheckedContinuation { waiter in
            fixWaiters.append(waiter)
            manager.requestLocation()
        }
    }

    nonisolated public func locationManagerDidChangeAuthorization(_ manager: CLLocationManager) {
        let status = manager.authorizationStatus
        Task { @MainActor in
            guard status != .notDetermined else { return }
            let waiters = permissionWaiters
            permissionWaiters = []
            for waiter in waiters { waiter.resume(returning: status) }
        }
    }

    nonisolated public func locationManager(_ manager: CLLocationManager, didUpdateLocations locations: [CLLocation]) {
        let answer: PositionAnswer =
            locations.last.map {
                .fix(
                    CheckInPosition(
                        latitude: $0.coordinate.latitude,
                        longitude: $0.coordinate.longitude,
                        accuracyMeters: $0.horizontalAccuracy
                    )
                )
            } ?? .unavailable
        Task { @MainActor in resume(answer) }
    }

    nonisolated public func locationManager(_ manager: CLLocationManager, didFailWithError error: any Error) {
        Task { @MainActor in resume(.unavailable) }
    }

    private func resume(_ answer: PositionAnswer) {
        let waiters = fixWaiters
        fixWaiters = []
        for waiter in waiters { waiter.resume(returning: answer) }
    }
}

/// The camera's scan of the unit's QR label (VisionKit): the first QR code it reads fills the form. Phones without the
/// scanner (and the simulator) type the label's code instead.
struct QRScanner: UIViewControllerRepresentable {
    let found: @MainActor (String) -> Void

    static var isAvailable: Bool { DataScannerViewController.isSupported && DataScannerViewController.isAvailable }

    func makeUIViewController(context: Context) -> DataScannerViewController {
        let scanner = DataScannerViewController(
            recognizedDataTypes: [.barcode(symbologies: [.qr])],
            qualityLevel: .balanced,
            isHighlightingEnabled: true
        )
        scanner.delegate = context.coordinator
        return scanner
    }

    func updateUIViewController(_ scanner: DataScannerViewController, context: Context) {
        if !scanner.isScanning { try? scanner.startScanning() }  // a camera the system refuses shows nothing to read
    }

    func makeCoordinator() -> Coordinator { Coordinator(found: found) }

    @MainActor
    final class Coordinator: NSObject, DataScannerViewControllerDelegate {
        let found: @MainActor (String) -> Void

        init(found: @escaping @MainActor (String) -> Void) {
            self.found = found
        }

        func dataScanner(_ dataScanner: DataScannerViewController, didAdd addedItems: [RecognizedItem], allItems: [RecognizedItem]) {
            for item in addedItems {
                if case let .barcode(code) = item, let value = code.payloadStringValue {
                    found(value)
                    return
                }
            }
        }
    }
}
#else
import SwiftUI

/// The package's tests build for macOS, which has no scanner: the code is typed.
struct QRScanner: View {
    let found: @MainActor (String) -> Void
    static let isAvailable = false

    var body: some View { EmptyView() }
}
#endif

extension View {
    /// A code field: typed as it is, without a capital or a correction.
    func codeEntry() -> some View {
        #if os(iOS)
        textInputAutocapitalization(.never).autocorrectionDisabled()
        #else
        autocorrectionDisabled()
        #endif
    }
}
