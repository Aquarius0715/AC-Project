import ACShared
import SwiftUI

/// The Customer app (DEC-112): the sign-in, then the customer's ACs, notifications and home-or-away location.
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
        content.tint(Brand.primary).id(session.language)
    }

    @ViewBuilder private var content: some View {
        switch session.state {
        case .signedIn:
            TabView {
                UnitsView(model: UnitsModel(client: session.client), session: session)
                    .tabItem { Label(tr("Air conditioners"), systemImage: "air.conditioner.horizontal") }
                NotificationsView(model: NotificationsModel(client: session.client))
                    .tabItem { Label(tr("Notifications"), systemImage: "bell") }
                PresenceView(model: presence)
                    .tabItem { Label(tr("Location"), systemImage: "location") }
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
            List {
                if let message = model.units.message {
                    Text(verbatim: message).foregroundStyle(.red).accessibilityIdentifier("units.message")
                }
                ForEach(model.units.value ?? []) { unit in
                    NavigationLink(value: unit.id) {
                        VStack(alignment: .leading, spacing: 4) {
                            Text(verbatim: unit.name).font(.headline)
                            HStack(spacing: 8) {
                                Text(verbatim: unit.power)
                                Text(verbatim: unit.connection)
                            }
                            .font(.subheadline)
                        }
                        .accessibilityElement(children: .combine)
                    }
                    .accessibilityIdentifier("unit.\(unit.id)")
                }
                if case .loaded([]) = model.units {
                    Text(verbatim: tr("No ACs in your account yet."))
                }
                Section {
                    Button(tr("Sign out")) { Task { await session.signOut() } }.accessibilityIdentifier("signOut")
                }
            }
            .overlay { if case .loading = model.units { ProgressView() } }
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
