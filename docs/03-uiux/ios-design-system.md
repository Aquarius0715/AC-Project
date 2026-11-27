# iOS design system: AC Customer and AC Technician

Version: 0.1.0 / Created: 2026-11-27 / Status: as built (IR884, DEC-112) / Code: `service/ios/ACKit/Sources/ACShared/DesignSystem`

The two iOS apps share one design system, built on Apple's [Human Interface Guidelines](https://developer.apple.com/design/human-interface-guidelines/) and SwiftUI's own controls. It is the iOS counterpart of the web apps' tokens and shared components ([UX-04](UIUXSpecification.md#ux-04-design-tokens-based-on-the-reference-design), [UX-05](UIUXSpecification.md#ux-05-shared-component-contracts)): the same brand blue and status colours, so an app and a page read as one service, but with the platform's navigation, type and controls. This document is the reference for every new screen; where it and the code differ, the code is right and this document is fixed (the implementation is the source of truth).

## 1. Principles

1. **The system first.** A SwiftUI control (Button, Toggle, Picker, TextField, NavigationStack, TabView, sheet, confirmationDialog, refreshable) is used as it is, with one of the styles below. A shared component exists only where several screens repeat the same composition (§4); a wrapper that only renames a system control is not added.
2. **Named by use, defined once.** A screen names a colour, a step of spacing, a radius, a symbol and a button kind by its use (`Palette.critical`, `Spacing.l`, `Symbol.warning`, `.primaryAction()`), never by its value. SwiftLint refuses the values themselves outside `DesignSystem/` (§7).
3. **Accessible by construction.** Every screen passes Xcode's accessibility audit (contrast, Dynamic Type, hit areas, labels) in its UI tests. The rules below record what the audit taught: they are not optional styling.
4. **Quiet.** No shadows, gradients, custom fonts or decorative animation. Hierarchy comes from the grouped background, the cards on it and the text styles.

## 2. Tokens

### Colours (`Palette.swift`)

Defined in code as dynamic colours of both appearances, not in an asset catalog: the package's unit tests build with `swift test` for macOS, which does not compile asset catalogs, and the tests measure every value's contrast (`DesignSystemTests.swift`). Light values are the web's (`service/web/shared/globals.css`).

| Token | Light | Dark | Use |
|---|---|---|---|
| `Palette.accent` | #005BEA | #5A9BFF | The apps' tint: links, icons, switches, the selected item, the unread dot |
| `Palette.primaryFill` | #005BEA | #005BEA | The fill of a primary button (white words 5.6:1 in both appearances) |
| `Palette.ok` | #166534 | #4ADE80 | Done, confirmed, connected |
| `Palette.warn` | #9A3412 | #FB923C | Needs attention soon |
| `Palette.critical` | #B91C1C | #F87171 | Failed, blocked, offline, destructive |
| `Palette.unknown` | #475569 | #A3B1C4 | Unknown or missing data; a muted status |
| `Palette.background` | systemGroupedBackground | systemGroupedBackground | The screen under the cards |
| `Palette.surface` | secondarySystemGroupedBackground (white) | secondarySystemGroupedBackground (#1C1C1E) | A card |
| text | `.primary` | `.primary` | All words. The system's grey (`.secondary`) is not used for words: it is below 4.5:1 on a card, and the audit refuses it; hierarchy comes from the text style instead |

Every text colour keeps 4.5:1 on the card and on the screen background in both appearances (the unit test fails otherwise). The dark accent is lighter than the light one because #005BEA on black is 3.4:1; a primary button keeps #005BEA because white words on the lighter blue would be 2.9:1. A new colour is added to `Swatch` and `Palette` with its use, both values, and a line in the contrast test.

### Typography

The system font and the system text styles only, so Dynamic Type scales every word; no custom font (none is needed for the four display languages, and a custom one would need its own Dynamic Type metrics).

| Style | Use |
|---|---|
| `.largeTitle` (bold) | The sign-in screen's app name only. Screens use inline navigation titles (`inlineTitle()`): a large title is clipped at the largest text sizes |
| `.title3` (bold) | A value the screen is about (the setpoint) |
| `.headline` | A card's heading (`SectionHeading`), a row's title, an emphasised value |
| `.body` | Default text, a fact's value |
| `.callout` | A message under the sign-in button |
| `.subheadline` | A row's detail line, a label above an emphasised value |
| `Font.factLabel` (`.subheadline` semibold) | The label above a fact's value, a row's status line |
| `.footnote` | When, how many, a hint, a note under a group |

`.caption` is not used: it is below the 12 pt minimum of the UIUX rules (UX-04) at the default size.

### Spacing, radius, size (`Tokens.swift`)

| Token | Value | Use |
|---|---|---|
| `Spacing.xxs` | 2 | The lines of one item (a label and its value) |
| `Spacing.xs` | 4 | An item's title and its detail lines |
| `Spacing.s` | 8 | The controls or items of a group |
| `Spacing.m` | 12 | The blocks inside a card |
| `Spacing.l` | 16 | A card's padding, between cards, the screen's margins |
| `Spacing.xl` | 24 | Around a screen of its own (sign-in) |
| `Radius.card` | 12 | A card (continuous corners) |
| `Radius.control` | 8 | A surface inside a card (photo, signature pad) |
| `Size.minTap` | 44 | The smallest side of a custom tap target (HIG) |

No shadow and no material: a card is told apart by its surface colour on the grouped background, as the system's grouped lists are, and text over a translucent material fell below the audit's contrast (the navigation bar is solid, `cardScreen()`).

### Symbols (`Symbol.swift`)

SF Symbols only, named by meaning: `Image(symbol: .warning)`, `Label(title, symbol: .photo)`. One meaning has one symbol on every screen; the outline style is the default and a filled symbol marks a state (unread). Symbols take the text style's size and weight of their line (no fixed point sizes), so they scale with Dynamic Type. A symbol is decorative when its words say the same (hidden from VoiceOver); a symbol alone (the history's failed mark) carries an accessibility label. A new symbol is added to `Symbol` with its meaning; the unit test checks that the system has it.

### Tones (`Tone.swift`)

`info`, `ok`, `warn`, `critical`, `muted` — the web badges' tones. A tone gives a colour and a symbol; a status always has its words too, so colour is never the only sign. The models (`JobsModel.Row.Tone`, `JobDetailModel.Banner.Tone`) use this one type.

## 3. Buttons (`ActionsView.swift`)

| Kind | Code | Use |
|---|---|---|
| Primary | `.primaryAction()` | The one action a screen or card is for (Sign in, Start job, Submit, Try again). At most one in view. Also the chosen one of a row of filter or choice buttons, with the selected trait (where a segmented Picker does not fit) |
| Secondary | `.secondaryAction()` | Every other action, an unchosen filter |
| Link | `.linkAction()` | A NavigationLink shown as a button |
| Destructive | `.destructiveAction()` with `role: .destructive` | Ends or removes something; one that removes data asks first in a confirmationDialog whose safe choice is Cancel |
| Plain | `.buttonStyle(.plain)` | A whole card that is tappable (a notification) |

No toolbar buttons for actions in the content (the audit refused their contrast on iOS 26's glass bar); a sheet's Close/Cancel is the exception. A disabled button stays visible with the system's dimming, and the reason is written beside it when it is not obvious (offline, sending).

## 4. Shared components

| Component | File | Responsibility |
|---|---|---|
| `Card(title) { … }` | `CardView.swift` | A group of a screen: optional `SectionHeading`, content, surface colour, `Spacing.l` padding |
| `.cardScreen()` | `CardView.swift` | A screen of cards: ScrollView (not List: a List leaves rows out of view out of the audit), grouped background, solid navigation bar, keyboard Done, drag to dismiss the keyboard |
| `SectionHeading` | `SectionHeadingView.swift` | A card's heading: headline, body colour, header trait |
| `Fact`, `Facts` | `FactsView.swift` | A label and value (and note) stacked, one VoiceOver element; `isEmphasized` for the few facts a screen is about |
| `DisclosureRow` | `DisclosureRowView.swift` | The label of a row that opens a screen: title, status line, details, chevron; inside a NavigationLink |
| `StatusLabel` | `StatusView.swift` | A status line with its tone's symbol and colour; `StatusLabel.fieldError` under a field |
| `.loadingOverlay(_:)` | `StatusView.swift` | The spinner of a first read; reloads keep the content (pull to refresh) |
| `inlineTitle()`, `hideTabBar()` | `SectionHeadingView.swift` | Inline navigation title; a pushed detail without the tab bar |

Each component file has `#Preview`s; check them in both appearances and at an accessibility text size before changing a component.

Not built, on purpose: a button type (the system Button with a style is enough), a text-field wrapper (TextField with `.roundedBorder` and its error as `StatusLabel.fieldError`), a search field (`.searchable` when a list needs it), a confirmation dialog (the system `.confirmationDialog`), an empty-state view (`ContentUnavailableView` draws its description in the system grey, below the audit's contrast: an empty list is a Card with one sentence of what will appear there).

A new shared component is added when the same composition appears on two screens or more, has one clear responsibility, takes plain values (not a model), and comes with previews. Otherwise the screen composes system views with the tokens.

## 5. UI/UX rules for screens

As built in IR884 every screen takes its values from the tokens; the move of the existing screens onto `StatusLabel`, `DisclosureRow` and the send haptics follows screen by screen (the Customer app, then the Technician app). A new screen follows these rules from the start.

**Navigation.** A TabView of three tabs at most per app, each tab its own NavigationStack; detail screens are pushed (`navigationDestination`) and hide the tab bar. A sheet is for a short task that returns to the same screen (check in, add a part, sign off), with Cancel/Close; a full-screen cover only for the camera. Back is always the system's back button and edge swipe; a screen never navigates by itself after an action. Titles are inline.

**Interaction.** Every action shows its state where it was tapped: a spinner or the waiting line while sending, then the outcome as a `StatusLabel` (ok or critical) in the same card; nothing is reported only by a toast. Controls are disabled while their write is sent. Destructive actions use the destructive style and role, and confirm when they lose data. Pull to refresh on every screen that reads. Swipe actions are not used (a hidden gesture with no visible equivalent).

**Layout.** One card per topic, the most important first (what the status asks of the user, then the object, then its history). The content respects the safe areas (no `ignoresSafeArea` but the camera). Portrait iPhone only (`TARGETED_DEVICE_FAMILY` 1); widths follow the screen with `Spacing.l` margins. A label sits above its value, not beside it, so the largest text sizes fit.

**Forms.** Fields in a card, each with its label; the error of a field under it (`StatusLabel.fieldError`) from the Core API's field refusal, the summary of blocking issues above the submit button. A field that grows uses `TextField(axis: .vertical)`; the keyboard has Done and is dismissed by dragging; `@FocusState` moves to the first field to fix where a form has several.

**Accessibility.** Every screen is covered by a UI test that runs the accessibility audit in the light and the dark appearance, up to the largest Dynamic Type sizes. Custom tap targets are at least `Size.minTap`. Icon-only buttons have a label; decorative symbols are hidden; a row is one element (`accessibilityElement(children: .combine)`), a heading has the header trait, a selected filter the selected trait. Every element a test drives has an `accessibilityIdentifier`.

**Animation and feedback.** The system's own transitions only (navigation, sheets, disclosure). A success or failure of a send gives the system's haptic (`sensoryFeedback`), which follows the user's settings. Nothing moves on its own except a progress indicator and the waiting countdown; with Reduce Motion the system already replaces its transitions.

## 6. External libraries

None. Each candidate was weighed against the standard APIs (iOS 17.4):

| Library | Would give | Decision |
|---|---|---|
| SwiftUIX | Extra views and modifiers | No: the standard views cover the screens; a large surface to keep in step with each iOS |
| SwiftUI-Introspect | Access to the UIKit views under SwiftUI | No: depends on private view hierarchies that change with iOS releases |
| Lottie | Vector animations | No: decorative animation is against §1; adds a large binary |
| Kingfisher / Nuke | Image download and cache | No: the only images are a report's photos, already in memory |
| Swift Charts | Charts | Already used (Apple's framework) |
| AppAuth | OAuth | Not needed: `ASWebAuthenticationSession` with PKCE (`Authenticator.swift`) |

A library is added only when the standard APIs cannot do the job, it supports the deployment target, it is maintained, its license is MIT/Apache/BSD, and the gain outweighs a dependency in both apps; the decision is recorded here.

## 7. Changing the design system

- A colour, step, radius or symbol changes in `DesignSystem/` only; the screens follow. A new value is added there with its use; a value no screen uses is removed.
- SwiftLint enforces it outside `DesignSystem/` (`service/ios/.swiftlint.yml`): `sf_symbol_name` (no symbol names), `palette_colour` (no system colour literals or `Color(red:…)`), `primary_action_style` (no `.borderedProminent`), `spacing_token` (no number in `spacing:`, `.padding(…)`, `cornerRadius:`).
- `DesignSystemTests.swift` holds the contrast of every text colour in both appearances, every symbol's existence and every tone's mapping at 100 % coverage.
- A change that alters how screens look runs both apps' UI tests (`sh service/ios/scripts/check.sh ui <scheme>`) and keeps their screenshots in the PR.
