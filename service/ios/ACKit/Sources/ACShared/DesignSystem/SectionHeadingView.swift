import SwiftUI

extension View {
    /// The screen's title in the navigation bar's line: a large title is clipped at the largest text sizes (the
    /// accessibility audit). The package's tests build for macOS, which has no such bar.
    public func inlineTitle() -> some View {
        #if os(iOS)
        navigationBarTitleDisplayMode(.inline)
        #else
        self
        #endif
    }
}

extension View {
    /// A pushed detail screen without the tab bar: the bar's translucent background over the screen's last lines is
    /// below the contrast the accessibility audit asks for.
    public func hideTabBar() -> some View {
        #if os(iOS)
        toolbar(.hidden, for: .tabBar)
        #else
        self
        #endif
    }
}

/// A list section's heading in the body colour: the system's grey heading is below the contrast the accessibility audit
/// asks for.
public struct SectionHeading: View {
    let title: String

    public init(_ title: String) {
        self.title = title
    }

    public var body: some View {
        Text(verbatim: title)
            .font(.headline)
            .foregroundColor(.primary)
            .textCase(nil)
            .accessibilityAddTraits(.isHeader)
    }
}
