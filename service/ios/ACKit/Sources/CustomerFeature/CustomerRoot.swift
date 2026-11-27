import ACShared
import SwiftUI

/// The Customer app (DEC-112): the sign-in, then the customer's ACs, notifications and settings (home-or-away, sign-out).
public struct CustomerRoot: View {
    let session: SessionModel
    let presence: PresenceModel
    let web: WebSignIn?

    /// `web` replaces the provider's page (the stubbed UI tests).
    public init(session: SessionModel, presence: PresenceModel, web: WebSignIn? = nil) {
        self.session = session
        self.presence = presence
        self.web = web
    }

    public var body: some View {
        // the membership's language rebuilds the screens in it once preferences.get has answered
        content.tint(Palette.accent).id(session.language)
    }

    @ViewBuilder private var content: some View {
        switch session.state {
        case .signedIn:
            TabView {
                UnitsView(model: UnitsModel(client: session.client), session: session)
                    .tabItem { Label(tr("Air conditioners"), symbol: .airConditioner) }
                NotificationsView(model: NotificationsModel(client: session.client))
                    .tabItem { Label(tr("Notifications"), symbol: .notifications) }
                PresenceView(model: presence) { await session.signOut() }
                    .tabItem { Label(tr("Settings"), symbol: .settings) }
            }
            .task {
                await session.applyPreferences()
                await presence.resume()
            }
        case .signedOut, .signingIn:
            SignInView(title: tr("Customer"), model: session, web: web)
                .task { await presence.pause() }
        }
    }
}

struct UnitsView: View {
    @State private var model: UnitsModel
    let session: SessionModel

    init(model: UnitsModel, session: SessionModel) {
        _model = State(initialValue: model)
        self.session = session
    }

    var body: some View {
        NavigationStack {
            // cards in a scroll view, as every screen (CardView.swift): a List leaves the rows out of view out of the audit
            Group {
                if let message = model.units.message {
                    Card { StatusLabel(message, tone: .critical).accessibilityIdentifier("units.message") }
                }
                let units = model.units.value ?? []
                if !units.isEmpty {
                    Card {
                        ForEach(units) { unit in
                            NavigationLink(value: unit.id) {
                                DisclosureRow(title: unit.name, details: [unit.power, unit.connection])
                                    .accessibilityElement(children: .combine)
                            }
                            .accessibilityIdentifier("unit.\(unit.id)")
                        }
                    }
                }
                if case .loaded([]) = model.units {
                    Card { Text(verbatim: tr("No ACs in your account yet.")) }
                }
            }
            .cardScreen()
            .loadingOverlay(model.units.isLoading)
            .navigationTitle(Text(verbatim: tr("Air conditioners")))
            .inlineTitle()
            .refreshable { await model.load() }
            .task { await model.load() }
            .navigationDestination(for: String.self) { id in
                UnitDetailView(model: UnitControlModel(unitId: id, client: session.client, me: session.membershipId))
            }
        }
    }
}
