import ACKit
import ACShared
import PencilKit
import PhotosUI
import SwiftUI

/// Add part: a catalog part or a name, how many, where it came from, and the checklist item it replaces.
struct PartSheet: View {
    @Bindable var model: ReportModel
    let close: () -> Void
    @State private var form = PartForm()

    var body: some View {
        NavigationStack {
            Group {
                Card(tr("Part (catalog)")) {
                    Picker(selection: $form.code) {
                        Text(verbatim: tr("Not in the catalog…")).tag(String?.none)
                        ForEach(model.parts, id: \.code) { part in
                            Text(verbatim: PartForm.label(part)).tag(String?.some(part.code))
                        }
                    } label: {
                        Text(verbatim: tr("Part (catalog)"))
                    }
                    .pickerStyle(.menu)
                    .accessibilityIdentifier("part.catalog")
                    if form.code == nil {
                        TextField(tr("Part name"), text: $form.name, axis: .vertical)
                            .textFieldStyle(.roundedBorder)
                            .accessibilityIdentifier("part.name")
                    }
                    Stepper(value: $form.quantity, in: 1...999) {
                        Text(verbatim: "\(tr("Qty")) \(form.quantity)")
                    }
                    .accessibilityIdentifier("part.quantity")
                    Picker(selection: $form.source) {
                        ForEach(PartSource.allCases, id: \.self) { Text(verbatim: Checklist.label($0)).tag($0) }
                    } label: {
                        Text(verbatim: tr("Source"))
                    }
                    .pickerStyle(.menu)
                    Picker(selection: $form.replaces) {
                        Text(verbatim: tr("None")).tag(ComponentKey?.none)
                        ForEach(model.draft.items, id: \.key) { Text(verbatim: Checklist.label($0.key)).tag(ComponentKey?.some($0.key)) }
                    } label: {
                        Text(verbatim: tr("Replaces checklist item (optional)"))
                    }
                    .pickerStyle(.menu)
                    if let error = form.error(catalog: model.parts) { Text(verbatim: error).accessibilityIdentifier("part.error") }
                }
                Card {
                    HStack {
                        Button(tr("Add")) {
                            guard let part = form.part(catalog: model.parts) else {
                                form.isChecked = true
                                return
                            }
                            model.edit { $0.parts.append(part) }
                            close()
                        }
                        .primaryAction()
                        .accessibilityIdentifier("part.save")
                        Button(tr("Cancel"), action: close).secondaryAction()
                    }
                }
            }
            .cardScreen()
            .navigationTitle(Text(verbatim: tr("Add part")))
            .inlineTitle()
        }
    }
}

/// A refrigerant record: the kind, the cylinder, recovered and charged kilograms and the leak check.
struct RefrigerantSheet: View {
    @Bindable var model: ReportModel
    let close: () -> Void
    @State private var form = RefrigerantForm()

    var body: some View {
        NavigationStack {
            Group {
                Card(tr("Refrigerant")) {
                    Picker(selection: $form.kind) {
                        ForEach(RefrigerantRecordRefrigerant.allCases, id: \.self) { Text(verbatim: $0.rawValue).tag($0) }
                    } label: {
                        Text(verbatim: tr("Refrigerant"))
                    }
                    .pickerStyle(.menu)
                    TextField(tr("Cylinder ID"), text: $form.cylinder, axis: .vertical).textFieldStyle(.roundedBorder)
                        .accessibilityIdentifier("refrigerant.cylinder")
                    TextField(tr("Recovered (kg)"), text: $form.recovered, axis: .vertical).textFieldStyle(.roundedBorder)
                        .accessibilityIdentifier("refrigerant.recovered")
                    TextField(tr("Charged (kg)"), text: $form.charged, axis: .vertical).textFieldStyle(.roundedBorder)
                        .accessibilityIdentifier("refrigerant.charged")
                    Picker(selection: $form.leakCheck) {
                        ForEach(RefrigerantRecordLeakCheck.allCases, id: \.self) { Text(verbatim: Checklist.label($0)).tag($0) }
                    } label: {
                        Text(verbatim: tr("Leak check"))
                    }
                    .pickerStyle(.menu)
                    if form.isChecked, let error = form.error { Text(verbatim: error).accessibilityIdentifier("refrigerant.error") }
                }
                Card {
                    HStack {
                        Button(tr("Add")) {
                            form.isChecked = true
                            guard let record = form.record else { return }
                            model.edit { $0.refrigerant.append(record) }
                            close()
                        }
                        .primaryAction()
                        .accessibilityIdentifier("refrigerant.save")
                        Button(tr("Cancel"), action: close).secondaryAction()
                    }
                }
            }
            .cardScreen()
            .navigationTitle(Text(verbatim: tr("Refrigerant")))
            .inlineTitle()
        }
    }
}

/// The customer's sign-off: the signer's name and the signature drawn on the phone, or why the customer could not sign
/// with a site photo.
struct SignOffSheet: View {
    @Bindable var model: ReportModel
    let close: () -> Void
    @State private var name = ""
    @State private var isAbsent = false
    @State private var reason = ""
    @State private var drawing = PKDrawing()
    @State private var picked: PhotosPickerItem?
    @State private var sitePhoto: NewPhoto?

    var body: some View {
        NavigationStack {
            Group {
                Card(tr("Signer name")) {
                    TextField(tr("Signer name"), text: $name, axis: .vertical).textFieldStyle(.roundedBorder)
                        .accessibilityIdentifier("signOff.name")
                }
                Card(tr("How")) {
                    HStack {
                        choice(false, tr("Signature"), id: "signOff.method.sign")
                        choice(true, tr("Customer not available"), id: "signOff.method.absent")
                    }
                    if isAbsent {
                        TextField(tr("Why the customer could not sign"), text: $reason, axis: .vertical)
                            .textFieldStyle(.roundedBorder)
                            .accessibilityIdentifier("signOff.reason")
                        PhotosPicker(selection: $picked, matching: .images) {
                            Label(sitePhoto.map(\.name) ?? tr("Site photo (JPEG/PNG)"), symbol: .photo)
                        }
                        .secondaryAction()
                    } else {
                        SignaturePad(drawing: $drawing)
                            .frame(height: 180)
                            .clipShape(RoundedRectangle(cornerRadius: Radius.control))
                            .overlay(RoundedRectangle(cornerRadius: Radius.control).stroke(.secondary))
                        HStack {
                            Label(tr("Sign here"), symbol: .signature).font(.footnote)
                            Spacer()
                            Button(tr("Clear")) { drawing = PKDrawing() }.secondaryAction().disabled(drawing.strokes.isEmpty)
                        }
                    }
                }
                Card {
                    if let error = model.signOffError { Text(verbatim: error).accessibilityIdentifier("signOff.error") }
                    if let refusal = model.refusal { Text(verbatim: refusal) }
                    HStack {
                        Button(tr("Save sign-off")) {
                            Task {
                                let signature = isAbsent ? nil : SignaturePad.png(drawing)
                                let saved = await model.signOff(
                                    signer: name,
                                    signature: signature,
                                    absentReason: isAbsent ? reason : nil,
                                    sitePhoto: isAbsent ? sitePhoto : nil
                                )
                                if saved { close() }
                            }
                        }
                        .primaryAction()
                        .disabled(model.isSending)
                        .accessibilityIdentifier("signOff.save")
                        Button(tr("Cancel"), action: close).secondaryAction()
                    }
                    Text(verbatim: tr("The sign-off is bound to this report version; saving the draft again clears it.")).font(.footnote)
                }
            }
            .cardScreen()
            .navigationTitle(Text(verbatim: tr("Customer sign-off")))
            .inlineTitle()
            .onChange(of: picked) { _, item in
                guard let item else { return }
                Task {
                    guard let data = try? await item.loadTransferable(type: Data.self) else { return }  // an unreadable pick adds nothing
                    sitePhoto = PhotoEncoding.jpeg(data, name: "site-photo")
                }
            }
        }
    }

    private func choice(_ absent: Bool, _ title: String, id: String) -> some View {
        Group {
            if isAbsent == absent {
                Button(title) { isAbsent = absent }.primaryAction()
            } else {
                Button(title) { isAbsent = absent }.secondaryAction()
            }
        }
        .accessibilityIdentifier(id)
        .accessibilityAddTraits(isAbsent == absent ? .isSelected : [])
    }
}
