import SwiftUI

/// A reading, a setting or a detail: its name, its value and an optional note (when it was observed), stacked so that
/// every size of text fits (a label beside its value is cut at the largest sizes). VoiceOver reads it as one element.
public struct Fact: View {
    let label: String
    let value: String
    let note: String?
    let isEmphasized: Bool

    /// `isEmphasized` makes the value the line the eye goes to (a room's temperature), for the few facts a screen is about.
    public init(label: String, value: String, note: String? = nil, isEmphasized: Bool = false) {
        self.label = label
        self.value = value
        self.note = note
        self.isEmphasized = isEmphasized
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: Spacing.xxs) {
            Text(verbatim: label).font(isEmphasized ? .subheadline : .factLabel)
            Text(verbatim: value).font(isEmphasized ? .headline : .body)
            if let note { Text(verbatim: note).font(.footnote) }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityElement(children: .combine)
    }
}

/// Label and value lines of a card (the web's SummaryList).
public struct Facts: View {
    let rows: [(String, String)]

    public init(rows: [(String, String)]) {
        self.rows = rows
    }

    public var body: some View {
        ForEach(Array(rows.enumerated()), id: \.offset) { _, row in
            Fact(label: row.0, value: row.1)
        }
    }
}

#Preview("Facts") {
    Card("Unit") {
        Fact(label: "Room temperature (measured)", value: "26.5°C", note: "observed 10:42", isEmphasized: true)
        Facts(rows: [("Model", "Daikin FTKF25"), ("Serial", "SN-0001")])
    }
    .padding(Spacing.l)
    .background(Palette.background)
}

#Preview("Facts, dark, largest text") {
    Card("Unit") {
        Fact(label: "Room temperature (measured)", value: "26.5°C", note: "observed 10:42", isEmphasized: true)
        Facts(rows: [("Model", "Daikin FTKF25"), ("Serial", "SN-0001")])
    }
    .padding(Spacing.l)
    .background(Palette.background)
    .preferredColorScheme(.dark)
    .dynamicTypeSize(.accessibility3)
}
