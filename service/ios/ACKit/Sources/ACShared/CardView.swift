import SwiftUI

#if os(iOS)
import UIKit
#endif

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
    /// The cards of a detail screen in a scroll view on the grouped background, under a navigation bar of that solid
    /// colour: text scrolled under the bar's translucent material is below the contrast the accessibility audit asks for.
    public func cardScreen() -> some View {
        ScrollView {
            VStack(spacing: 16) { self }.padding(16)
        }
        // a drag down puts the keyboard away, so the screen under it can be read again (and audited); not any scroll:
        // the scroll that brings a focused field into view would close its keyboard again
        .scrollDismissesKeyboard(.interactively)
        .background(Color.groupedBackground)
        .solidNavigationBar()
        .clearBottomEdge()
        .keyboardDone()
    }

    /// A Done button over the keyboard: Return in a field that grows adds a line, so it is the way to put the keyboard
    /// away and see the screen under it again.
    @ViewBuilder
    private func keyboardDone() -> some View {
        #if os(iOS)
        toolbar {
            ToolbarItemGroup(placement: .keyboard) {
                Spacer()
                Button(tr("Done")) {
                    UIApplication.shared.sendAction(#selector(UIResponder.resignFirstResponder), to: nil, from: nil, for: nil)
                }
                .accessibilityIdentifier("keyboard.done")
            }
        }
        #else
        self
        #endif
    }

    /// No soft edge at the foot of the screen (iOS 26 blurs what scrolls under the home indicator): a button there kept
    /// its words below the contrast the accessibility audit asks for.
    @ViewBuilder
    private func clearBottomEdge() -> some View {
        if #available(iOS 26, macOS 26, *) {
            scrollEdgeEffectHidden(true, for: .bottom)
        } else {
            self
        }
    }

    private func solidNavigationBar() -> some View {
        #if os(iOS)
        toolbarBackground(Color.groupedBackground, for: .navigationBar).toolbarBackground(.visible, for: .navigationBar)
        #else
        self
        #endif
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
