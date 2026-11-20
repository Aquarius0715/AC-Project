import ACShared
import SwiftUI

/// The Technician app (DEC-112): the sign-in, then the technician's jobs.
public struct TechnicianRoot: View {
    let session: SessionModel
    let web: WebSignIn?

    /// `web` replaces the provider's page (the stubbed UI tests).
    public init(session: SessionModel, web: WebSignIn? = nil) {
        self.session = session
        self.web = web
    }

    public var body: some View {
        content.tint(Brand.primary)
    }

    @ViewBuilder private var content: some View {
        switch session.state {
        case .signedIn:
            JobsView(model: JobsModel(client: session.client), session: session)
        case .signedOut, .signingIn:
            SignInView(title: tr("Technician"), model: session, web: web)
        }
    }
}

struct JobsView: View {
    @State private var model: JobsModel
    let session: SessionModel

    init(model: JobsModel, session: SessionModel) {
        _model = State(initialValue: model)
        self.session = session
    }

    var body: some View {
        NavigationStack {
            List {
                if let message = model.jobs.message {
                    Text(verbatim: message).foregroundStyle(.red).accessibilityIdentifier("jobs.message")
                }
                let jobs = model.jobs.value ?? []
                section(tr("Open jobs"), jobs.filter { !$0.isPast })
                section(tr("Job history"), jobs.filter(\.isPast))
                if case .loaded([]) = model.jobs {
                    Text(verbatim: tr("No jobs yet"))
                }
                Section {
                    Button(tr("Sign out")) { Task { await session.signOut() } }.accessibilityIdentifier("signOut")
                }
            }
            .overlay { if case .loading = model.jobs { ProgressView() } }
            .navigationTitle(Text(verbatim: tr("Jobs")))
            .inlineTitle()
            .refreshable { await model.load() }
            .task { await model.load() }
        }
    }

    @ViewBuilder
    private func section(_ title: String, _ jobs: [JobsModel.Row]) -> some View {
        if !jobs.isEmpty {
            Section {
                ForEach(jobs) { job in
                    VStack(alignment: .leading, spacing: 4) {
                        Text(verbatim: job.title).font(.headline)
                        Text(verbatim: job.status).font(.subheadline)
                        Text(verbatim: job.due).font(.subheadline)
                    }
                    .accessibilityElement(children: .combine)
                    .accessibilityIdentifier("job.\(job.id)")
                }
            } header: {
                SectionHeading(title)
            }
        }
    }
}
