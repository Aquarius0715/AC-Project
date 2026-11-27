import SwiftUI

/// The spacing scale of every stack and padding (docs/03-uiux/ios-design-system.md), on a 4 pt grid with a 2 pt step for
/// the lines of one fact. A screen takes a step from here, never a number.
public enum Spacing {
    /// Between the lines of one item: a label and its value, a title and its date.
    public static let xxs: CGFloat = 2
    /// Between an item's title and its detail lines.
    public static let xs: CGFloat = 4
    /// Between the controls or the items of a group.
    public static let s: CGFloat = 8
    /// Between the blocks inside a card.
    public static let m: CGFloat = 12
    /// A card's padding, between cards, the screen's margins.
    public static let l: CGFloat = 16
    /// Around a screen of its own (the sign-in).
    public static let xl: CGFloat = 24
}

/// Corner radii: the system's continuous corners, one per kind of surface.
public enum Radius {
    /// A card on the grouped background.
    public static let card: CGFloat = 12
    /// A smaller surface inside a card (a photo, a signature pad).
    public static let control: CGFloat = 8
}

/// Sizes the Human Interface Guidelines set.
public enum Size {
    /// The smallest side of a tap target.
    public static let minTap: CGFloat = 44
}

extension Font {
    /// The label above a value (a fact, a field's group): the subheadline, semibold.
    public static let factLabel: Font = .subheadline.weight(.semibold)
}
