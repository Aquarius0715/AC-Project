import SwiftUI

/// A card of a detail screen, as the web pages group a screen: a heading and its content on the grouped background.
/// A detail screen stacks its cards in a ScrollView, not a List: a List leaves the rows out of view out of the screen's
/// elements, so the accessibility audit cannot check them at the larger text sizes.
public struct Card<Content: View>: View {
    let title: String?
    let content: Content

    public init(_ title: String? = nil, @ViewBuilder content: () -> Content) {
        self.title = title
        self.content = content()
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            if let title { SectionHeading(title) }
            content
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(16)
        .background(.background, in: RoundedRectangle(cornerRadius: 12))
    }
}

extension View {
    /// The cards of a detail screen in a scroll view on the grouped background.
    public func cardScreen() -> some View {
        ScrollView {
            VStack(spacing: 16) { self }.padding(16)
        }
        .background(Color.groupedBackground)
    }
}

extension Color {
    /// The grouped background of the system (the cards sit on it).
    public static var groupedBackground: Color {
        #if os(iOS)
        Color(uiColor: .systemGroupedBackground)
        #else
        Color(nsColor: .windowBackgroundColor)
        #endif
    }
}
