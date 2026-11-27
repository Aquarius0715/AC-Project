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
        content.tint(Palette.accent).id(session.language)
    }

    @ViewBuilder private var content: some View {
        switch session.state {
        case .signedIn:
            TabView {
                JobsView(model: JobsModel(client: session.client), session: session, position: position)
                    .tabItem { Label(tr("Jobs"), symbol: .jobs) }
                DevicesView(model: DevicesModel(client: session.client))
                    .tabItem { Label(tr("Devices"), symbol: .devices) }
                NotificationsView(model: NotificationsModel(client: session.client))
                    .tabItem { Label(tr("Notifications"), symbol: .notifications) }
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
                        StatusLabel(line, tone: .warn, symbol: .message)
                            .accessibilityIdentifier("jobs.answer")
                    }
                }
                if !model.tiles.isEmpty {
                    Card {
                        Facts(rows: model.tiles.map { ($0.label, "\($0.value) · \($0.sub)") }).accessibilityIdentifier("overview.tiles")
                    }
                }
                let jobs = model.jobs.value ?? []
                section(tr("Open jobs"), jobs.filter { !$0.isPast })
                let past = jobs.filter(\.isPast)
                if !past.isEmpty {
                    // the past jobs on a screen of their own: the list is for the work to do
                    NavigationLink {
                        JobHistoryView(jobs: past)
                    } label: {
                        Label(tr("Job history") + " (\(past.count))", symbol: .history).frame(maxWidth: .infinity, alignment: .leading)
                    }
                    .linkAction()
                    .accessibilityIdentifier("jobs.history")
                }
                if case .loaded = model.jobs {
                    overviewSection(
                        tr("Alerts on my units ({n})", ["n": String(model.alerts.count)]),
                        model.alerts,
                        empty: tr("No open alerts on your units."),
                        note: tr("Only units in your assignments. Finishing a job does not resolve its alert.")
                    )
                    overviewSection(tr("My reports"), model.reports, empty: tr("No drafts, returned or submitted reports."), note: nil)
                }
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

    private func overviewSection(_ title: String, _ rows: [OverviewWords.Row], empty: String, note: String?) -> some View {
        Card(title) {
            if rows.isEmpty { Text(verbatim: empty) }
            ForEach(rows) { row in
                NavigationLink(value: row.jobId) {
                    DisclosureRow(title: row.title, details: ["\(row.badge) · \(row.sub)"])
                }
                .accessibilityIdentifier("overview.\(row.id)")
            }
            if let note { Text(verbatim: note).font(.footnote) }
        }
    }

    @ViewBuilder
    private func section(_ title: String, _ jobs: [JobsModel.Row]) -> some View {
        if !jobs.isEmpty { JobRows(title: title, jobs: jobs) }
    }
}

/// A card of job rows, each opening its job.
struct JobRows: View {
    let title: String
    let jobs: [JobsModel.Row]

    var body: some View {
        Card(title) {
            ForEach(jobs) { job in
                NavigationLink(value: job.id) {
                    DisclosureRow(title: job.title, status: job.badge, details: [job.line])
                }
                .accessibilityIdentifier("job.\(job.id)")
            }
        }
    }
}

/// The technician's past assignments (the jobs list keeps the open ones); a row opens through the jobs list's stack.
struct JobHistoryView: View {
    let jobs: [JobsModel.Row]

    var body: some View {
        JobRows(title: tr("Job history"), jobs: jobs)
            .cardScreen()
            .navigationTitle(Text(verbatim: tr("Job history")))
            .inlineTitle()
            .hideTabBar()
    }
}
