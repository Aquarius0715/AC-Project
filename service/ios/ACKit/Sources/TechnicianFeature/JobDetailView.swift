import ACKit
import ACShared
import SwiftUI

/// One assigned job (the web's technician job page): what the status asks, the job and its unit, the time on site.
struct JobDetailView: View {
    @State private var model: JobDetailModel
    @State private var sheet: Sheet?

    enum Sheet: String, Identifiable {
        case checkIn, cantMake
        var id: String { rawValue }
    }

    init(model: JobDetailModel) {
        _model = State(initialValue: model)
    }

    var body: some View {
        Group {
            if let message = model.content.message {
                Card { Text(verbatim: message).accessibilityIdentifier("job.message") }
            }
            switch model.content.value {
            case let .work(work)?: workCards(work)
            case let .history(snapshot)?: HistoryCard(history: JobWords.history(snapshot))
            case .notFound?:
                Card(tr("This page isn’t available")) {
                    Text(verbatim: tr("The page doesn’t exist, or your account can’t open it. Nothing was navigated automatically."))
                }
            case nil: ProgressView()
            }
        }
        .cardScreen()
        .navigationTitle(Text(verbatim: model.title))
        .inlineTitle()
        .hideTabBar()
        .task { await model.load() }
        .refreshable { await model.load() }
        .sheet(item: $sheet) { sheet in
            switch sheet {
            case .checkIn: CheckInView(model: model) { self.sheet = nil }
            case .cantMake: CantMakeView(model: model) { self.sheet = nil }
            }
        }
    }

    @ViewBuilder
    private func workCards(_ work: JobDetailModel.Work) -> some View {
        if let note = model.note {
            Card {
                Label {
                    Text(verbatim: note)
                } icon: {
                    Image(systemName: "checkmark.circle")
                }.accessibilityIdentifier("job.note")
            }
        }
        if let refusal = model.refusal {
            Card {
                Label {
                    Text(verbatim: refusal)
                } icon: {
                    Image(systemName: "exclamationmark.triangle")
                }
                .accessibilityIdentifier("job.refusal")
            }
        }
        ForEach(Array(model.banners.enumerated()), id: \.offset) { _, banner in
            Card { bannerContent(banner) }
        }
        reportCard
        jobCard(work)
        if let window = model.window {
            Card(tr("Work window")) { Text(verbatim: window.text).accessibilityIdentifier("job.window") }
        }
        Card(tr("Time on site")) {
            Facts(rows: model.timeRows)
            if model.isInWork {
                Button(model.isPaused ? tr("Resume work") : tr("Pause work")) {
                    Task { await model.setPaused(!model.isPaused) }
                }
                .secondaryAction()
                .disabled(model.isSending || !model.canPause)
                .accessibilityIdentifier("job.pause")
            }
            Text(
                verbatim: tr(
                    "Arrival inside the window counts toward the SLA. Times are part of the report and visible to the contractor and HQ."
                )
            )
            .font(.footnote)
        }
    }

    /// The job and its unit, with the unit's screen.
    private func jobCard(_ work: JobDetailModel.Work) -> some View {
        Card(tr("Job & unit")) {
            Facts(rows: model.facts)
            NavigationLink {
                TechUnitView(model: TechUnitModel(unitId: work.job.unitId, jobId: work.job.id, client: model.client))
            } label: {
                Label(tr("Unit"), systemImage: "air.conditioner.horizontal")
            }
            .linkAction()
            .accessibilityIdentifier("job.unit")
        }
    }

    @ViewBuilder private var reportCard: some View {
        Card(tr("Work report")) {
            if let report = model.report() {
                NavigationLink {
                    ReportView(model: report)
                } label: {
                    Label(tr("Open the work report"), systemImage: "doc.text")
                }
                .linkAction()
                .accessibilityIdentifier("job.report")
            } else {
                Text(verbatim: tr("The report opens when you check in at the site."))
            }
        }
    }

    @ViewBuilder
    private func bannerContent(_ banner: JobDetailModel.Banner) -> some View {
        Label {
            Text(verbatim: banner.text)
        } icon: {
            Image(systemName: icon(banner.tone)).foregroundStyle(color(banner.tone))
        }
        .accessibilityIdentifier("job.banner")
        switch banner.action {
        case .acknowledge?:
            HStack {
                Button(tr("Accept assignment")) { Task { await model.accept() } }
                    .buttonStyle(.borderedProminent)
                    .accessibilityIdentifier("job.accept")
                Button(tr("Can’t make this time…")) { sheet = .cantMake }
                    .secondaryAction()
                    .accessibilityIdentifier("job.cantMake")
            }
            .disabled(model.isSending)
        case let .checkIn(enabled)?:
            Button(tr("Start job (check in)")) { sheet = .checkIn }
                .buttonStyle(.borderedProminent)
                .disabled(model.isSending || !enabled)
                .accessibilityIdentifier("job.checkIn")
        case let .resumeRework(enabled)?:
            Button(tr("Resume rework")) { Task { await model.resumeRework() } }
                .buttonStyle(.borderedProminent)
                .disabled(model.isSending || !enabled)
                .accessibilityIdentifier("job.resumeRework")
        case nil:
            EmptyView()
        }
    }

    private func icon(_ tone: JobDetailModel.Banner.Tone) -> String {
        switch tone {
        case .info: "info.circle"
        case .ok: "checkmark.circle"
        case .warn: "exclamationmark.triangle"
        case .critical: "exclamationmark.octagon"
        }
    }

    private func color(_ tone: JobDetailModel.Banner.Tone) -> Color {
        switch tone {
        case .info: Brand.primary
        case .ok: .green
        case .warn: .orange
        case .critical: .red
        }
    }
}

/// Label and value lines of a card (the web's SummaryList).
struct Facts: View {
    let rows: [(String, String)]

    var body: some View {
        ForEach(Array(rows.enumerated()), id: \.offset) { _, row in
            VStack(alignment: .leading, spacing: 2) {
                Text(verbatim: row.0).font(.subheadline.weight(.semibold))
                Text(verbatim: row.1)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .accessibilityElement(children: .combine)
        }
    }
}

/// A job whose viewing window ended: what is left to know of it.
struct HistoryCard: View {
    let history: JobWords.History

    var body: some View {
        Card(history.title) {
            Text(verbatim: history.subtitle).font(.subheadline)
            Facts(rows: history.rows.map { ($0.label, $0.value) })
            Text(verbatim: history.note).font(.footnote).accessibilityIdentifier("job.history")
        }
    }
}
