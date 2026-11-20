import ACKit
import ACShared
import SwiftUI

/// The Technician app (DEC-112): the sign-in, then the technician's jobs and the notifications.
public struct TechnicianRoot: View {
    let session: SessionModel
    let web: WebSignIn?
    let position: any PositionSource

    /// `web` replaces the provider's page and `position` the phone's fix (the stubbed UI tests).
    public init(session: SessionModel, position: any PositionSource, web: WebSignIn? = nil) {
        self.session = session
        self.position = position
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
                JobsView(model: JobsModel(client: session.client), session: session, position: position)
                    .tabItem { Label(tr("Jobs"), systemImage: "wrench.and.screwdriver") }
                DevicesView(model: DevicesModel(client: session.client))
                    .tabItem { Label(tr("Devices"), systemImage: "sensor") }
                NotificationsView(model: NotificationsModel(client: session.client))
                    .tabItem { Label(tr("Notifications"), systemImage: "bell") }
            }
            .task { await session.applyPreferences() }
        case .signedOut, .signingIn:
            SignInView(title: tr("Technician"), model: session, web: web)
        }
    }
}

struct JobsView: View {
    @State private var model: JobsModel
    let session: SessionModel
    let position: any PositionSource

    init(model: JobsModel, session: SessionModel, position: any PositionSource) {
        _model = State(initialValue: model)
        self.session = session
        self.position = position
    }

    var body: some View {
        NavigationStack {
            // cards in a scroll view, not a List: the accessibility audit cannot check the rows a list leaves out
            Group {
                if let message = model.jobs.message {
                    Card { Text(verbatim: message).accessibilityIdentifier("jobs.message") }
                }
                if let line = model.answerLine {
                    Card {
                        Label {
                            Text(verbatim: line)
                        } icon: {
                            Image(systemName: "envelope").foregroundStyle(.orange)
                        }
                        .accessibilityIdentifier("jobs.answer")
                    }
                }
                let jobs = model.jobs.value ?? []
                section(tr("Open jobs"), jobs.filter { !$0.isPast })
                section(tr("Job history"), jobs.filter(\.isPast))
                if case .loaded([]) = model.jobs {
                    Card { Text(verbatim: tr("Jobs assigned to you appear here. Past assignments stay in their job history.")) }
                }
                if case .loading = model.jobs { ProgressView() }
                Button(tr("Sign out")) { Task { await session.signOut() } }
                    .secondaryAction()
                    .accessibilityIdentifier("signOut")
            }
            .cardScreen()
            .navigationTitle(Text(verbatim: tr("Jobs")))
            .inlineTitle()
            .navigationDestination(for: String.self) { id in
                JobDetailView(model: JobDetailModel(jobId: id, client: session.client, position: position))
            }
            .refreshable { await model.load() }
            .task { await model.load() }
        }
    }

    @ViewBuilder
    private func section(_ title: String, _ jobs: [JobsModel.Row]) -> some View {
        if !jobs.isEmpty {
            Card(title) {
                ForEach(jobs) { job in
                    NavigationLink(value: job.id) {
                        HStack {
                            VStack(alignment: .leading, spacing: 4) {
                                Text(verbatim: job.title).font(.headline)
                                Text(verbatim: job.badge).font(.subheadline.weight(.semibold))
                                Text(verbatim: job.line).font(.subheadline)
                            }
                            .multilineTextAlignment(.leading)
                            Spacer(minLength: 0)
                            Image(systemName: "chevron.right").accessibilityHidden(true)
                        }
                        .foregroundStyle(.primary)
                    }
                    .accessibilityIdentifier("job.\(job.id)")
                }
            }
        }
    }
}
