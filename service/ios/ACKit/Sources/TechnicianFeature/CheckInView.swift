import ACKit
import ACShared
import SwiftUI

/// Check in at site (DD-T13): the phone's fix, taken as the sheet opens, and the unit's QR label; a manual check-in
/// with a reason only when the label cannot be scanned or the phone has no fix.
struct CheckInView: View {
    @Bindable var model: JobDetailModel
    let close: () -> Void
    @State private var isScanning = false

    var body: some View {
        NavigationStack {
            Group {
                if model.checkIn.method == .locationQr {
                    Card(tr("Scanned unit QR")) { qr }
                    Card(tr("Your location")) { location }
                    // the fallback, out of the way of the usual check-in
                    method(.manual, tr("Manual (reason)"), id: "checkIn.method.manual")
                } else {
                    Card(tr("Reason (required)")) {
                        TextField(tr("No GPS signal in the basement plant room"), text: $model.checkIn.reason, axis: .vertical)
                            .lineLimit(3...6)
                            .textFieldStyle(.roundedBorder)
                            .accessibilityIdentifier("checkIn.reason")
                        error(.reason)
                    }
                    method(.locationQr, tr("Location + unit QR"), id: "checkIn.method.qr")
                }
                Card {
                    Text(verbatim: tr("Arrival must be inside your work window. Checking in records the arrival and starts the work."))
                        .font(.footnote)
                    if let refusal = model.refusal { Text(verbatim: refusal).accessibilityIdentifier("checkIn.refusal") }
                    HStack {
                        Button(tr("Check in & start")) {
                            Task { if await model.submitCheckIn() { close() } }
                        }
                        .primaryAction()
                        .disabled(model.isSending)
                        .accessibilityIdentifier("checkIn.submit")
                        Button(tr("Cancel"), action: close).secondaryAction().accessibilityIdentifier("checkIn.cancel")
                    }
                }
            }
            .cardScreen()
            .navigationTitle(Text(verbatim: tr("Check in at site")))
            .inlineTitle()
            .sheet(isPresented: $isScanning) { scanner }
            // the fix is taken as the sheet opens: the technician has nothing to tap for it
            .task { if model.checkIn.position == nil { await model.locate() } }
        }
    }

    /// The switch to the other way of checking in, as a link-styled button.
    private func method(_ method: JobsCheckInInputMethod, _ title: String, id: String) -> some View {
        Button(title) { model.checkIn.method = method }
            .linkAction()
            .accessibilityIdentifier(id)
    }

    @ViewBuilder private var location: some View {
        let label: String =
            if model.isLocating { tr("Locating…") } else if model.checkIn.position == nil { tr("Get my location") } else {
                tr("Locate again")
            }
        Text(verbatim: model.checkIn.positionHint).font(.footnote).accessibilityIdentifier("checkIn.positionHint")
        Button {
            Task { await model.locate() }
        } label: {
            Label(label, symbol: .location)
        }
        .secondaryAction()
        .disabled(model.isLocating)
        .accessibilityIdentifier("checkIn.locate")
        error(.position)
    }

    /// The camera first (on a phone that has one), the label's text below it for a label that cannot be scanned.
    @ViewBuilder private var qr: some View {
        if QRScanner.isAvailable {
            Button {
                isScanning = true
            } label: {
                Label(tr("Scan"), symbol: .scan).frame(maxWidth: .infinity)
            }
            .secondaryAction()
            .accessibilityIdentifier("checkIn.scan")
        }
        // a field that grows: one line is clipped at the larger text sizes
        TextField(tr("Scan the unit’s QR label"), text: $model.checkIn.qr, axis: .vertical)
            .lineLimit(1...3)
            .textFieldStyle(.roundedBorder)
            .codeEntry()
            .accessibilityIdentifier("checkIn.qr")
        error(.qr)
    }

    private var scanner: some View {
        NavigationStack {
            QRScanner { code in
                model.checkIn.qr = code
                isScanning = false
            }
            .ignoresSafeArea()
            .navigationTitle(Text(verbatim: tr("Scan the unit’s QR label")))
            .inlineTitle()
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button(tr("Cancel")) { isScanning = false } }
            }
        }
    }

    @ViewBuilder
    private func error(_ field: CheckInField) -> some View {
        if let text = model.checkInErrors[field] {
            StatusLabel.fieldError(text)
                .accessibilityIdentifier("checkIn.error.\(field.rawValue)")
        }
    }
}

/// Can't make this time: the reason for the coordinator and, optionally, a time the technician could do instead.
struct CantMakeView: View {
    @Bindable var model: JobDetailModel
    let close: () -> Void

    var body: some View {
        NavigationStack {
            Group {
                Card(tr("Reason (required)")) {
                    TextField(tr("Medical appointment on that morning"), text: $model.cantMake.reason, axis: .vertical)
                        .lineLimit(3...6)
                        .textFieldStyle(.roundedBorder)
                        .accessibilityIdentifier("cantMake.reason")
                    if let error = model.cantMakeError {
                        Text(verbatim: error).accessibilityIdentifier("cantMake.error")
                    }
                }
                Card {
                    Toggle(isOn: offersTime) { Text(verbatim: tr("A time you could do instead (optional)")) }
                        .accessibilityIdentifier("cantMake.offer")
                    if let date = model.cantMake.date {
                        DatePicker(
                            selection: Binding(get: { date }, set: { model.cantMake.date = $0 }),
                            displayedComponents: .date
                        ) { Text(verbatim: tr("Date")) }
                        Picker(selection: $model.cantMake.window) {
                            ForEach(CantMakeForm.windows, id: \.self) { Text(verbatim: $0).tag($0) }
                        } label: {
                            Text(verbatim: tr("Window"))
                        }
                        Text(verbatim: tr("Times in {zone}.", ["zone": Localizer.timeZone.identifier])).font(.footnote)
                    }
                }
                Card {
                    Text(verbatim: tr("Your coordinator reassigns the job or proposes a new time — the client must approve any change."))
                        .font(.footnote)
                    if let refusal = model.refusal { Text(verbatim: refusal).accessibilityIdentifier("cantMake.refusal") }
                    HStack {
                        Button(tr("Send to coordinator")) {
                            Task { if await model.sendCantMake() { close() } }
                        }
                        .primaryAction()
                        .disabled(model.isSending)
                        .accessibilityIdentifier("cantMake.send")
                        Button(tr("Cancel"), action: close).secondaryAction().accessibilityIdentifier("cantMake.cancel")
                    }
                }
            }
            .cardScreen()
            .navigationTitle(Text(verbatim: tr("Can’t make this time")))
            .inlineTitle()
        }
    }

    private var offersTime: Binding<Bool> {
        Binding(get: { model.cantMake.date != nil }, set: { model.cantMake.date = $0 ? Date() : nil })
    }
}
