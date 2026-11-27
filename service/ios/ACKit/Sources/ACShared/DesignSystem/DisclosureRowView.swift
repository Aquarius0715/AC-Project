import SwiftUI

/// The label of a row that opens another screen (a job, a unit, an alert): its title, an optional status line and the
/// detail lines, with the disclosure chevron. It goes inside a NavigationLink, which gives it its tap and its trait; the
/// words keep the body colour (a link's accent colour is below the contrast the audit asks for on a card).
public struct DisclosureRow: View {
    let title: String
    let status: String?
    let details: [String]

    public init(title: String, status: String? = nil, details: [String] = []) {
        self.title = title
        self.status = status
        self.details = details
    }

    public var body: some View {
        HStack(spacing: Spacing.s) {
            VStack(alignment: .leading, spacing: Spacing.xs) {
                Text(verbatim: title).font(.headline)
                if let status { Text(verbatim: status).font(.factLabel) }
                ForEach(Array(details.enumerated()), id: \.offset) { _, line in
                    Text(verbatim: line).font(.subheadline)
                }
            }
            .multilineTextAlignment(.leading)
            Spacer(minLength: 0)
            Image(symbol: .disclosure).font(.footnote.weight(.semibold)).accessibilityHidden(true)
        }
        .foregroundStyle(.primary)
        .contentShape(Rectangle())
    }
}

#Preview("Disclosure rows") {
    NavigationStack {
        Card("Open jobs") {
            NavigationLink(value: "job-1") {
                DisclosureRow(title: "Living room AC", status: "Not accepted", details: ["Dec 8, 9:00 – 11:00"])
            }
            NavigationLink(value: "job-2") { DisclosureRow(title: "Office AC", details: ["On · Online"]) }
        }
        .cardScreen()
    }
}

#Preview("Disclosure rows, dark, largest text") {
    NavigationStack {
        Card("Open jobs") {
            NavigationLink(value: "job-1") {
                DisclosureRow(title: "Living room AC", status: "Not accepted", details: ["Dec 8, 9:00 – 11:00"])
            }
            NavigationLink(value: "job-2") { DisclosureRow(title: "Office AC", details: ["On · Online"]) }
        }
        .cardScreen()
    }
    .preferredColorScheme(.dark)
    .dynamicTypeSize(.accessibility3)
}
