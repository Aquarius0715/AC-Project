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
            // a failed first read shows only its message, not a spinner that never ends
            case nil: if case .loading = model.content { ProgressView() }
            }
        }
        .cardScreen()
        .navigationTitle(Text(verbatim: model.title))
        .inlineTitle()
        .hideTabBar()
        .task { await model.load() }
        .refreshable { await model.load() }
        // a write's answer is felt as well as read
        .writeFeedback(note: model.note, refusal: model.refusal)
        // the one step the state asks for, at the foot of the screen where the thumb is; the others in its menu
        .safeAreaInset(edge: .bottom) { actionBar }
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
                StatusLabel(note, tone: .ok).accessibilityIdentifier("job.note")
            }
        }
        if let refusal = model.refusal {
            Card {
                StatusLabel(refusal, tone: .critical)
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
            Text(
                verbatim: tr(
                    "Arrival inside the window counts toward the SLA. Times are part of the report and visible to the contractor and HQ."
                )
            )
            .font(.footnote)
        }
    }

    /// The job and its unit, with the unit's screen (its readings, open alerts and diagnostics are there).
    private func jobCard(_ work: JobDetailModel.Work) -> some View {
        Card(tr("Job & unit")) {
            Facts(rows: model.facts)
            NavigationLink {
                TechUnitView(model: TechUnitModel(unitId: work.job.unitId, jobId: work.job.id, client: model.client))
            } label: {
                Label(tr("Unit"), symbol: .airConditioner)
            }
            .linkAction()
            .accessibilityIdentifier("job.unit")
        }
    }

    /// Before the check-in, where the report will be; afterwards the report is a step of the action bar.
    @ViewBuilder private var reportCard: some View {
        if !model.hasReport {
            Card(tr("Work report")) { Text(verbatim: tr("The report opens when you check in at the site.")) }
        }
    }

    /// The primary step of the job's state, full width, with the menu of its other steps.
    @ViewBuilder private var actionBar: some View {
        if let step = model.primaryStep, model.work != nil {
            HStack(spacing: Spacing.s) {
                stepButton(step)
                if !model.moreSteps.isEmpty {
                    Menu {
                        ForEach(model.moreSteps, id: \.self) { stepButton($0, inMenu: true) }
                    } label: {
                        Image(symbol: .more).frame(minWidth: Size.minTap, minHeight: Size.minTap)
                    }
                    .secondaryAction()
                    .accessibilityLabel(Text(verbatim: tr("More actions")))
                    .accessibilityIdentifier("job.more")
                }
            }
            .padding(Spacing.l)
            .background(Palette.surface)
            .disabled(model.isSending)
        }
    }

    /// The button of one step: in the bar, the full-width primary button; in the menu, its line.
    @ViewBuilder
    private func stepButton(_ step: JobDetailModel.Step, inMenu: Bool = false) -> some View {
        let button = Group {
            switch step {
            case .accept:
                button(tr("Accept assignment"), id: "job.accept", inMenu) { Task { await model.accept() } }
            case .cantMake:
                button(tr("Can’t make this time…"), id: "job.cantMake", inMenu) { sheet = .cantMake }
            case let .checkIn(enabled):
                button(tr("Start job (check in)"), id: "job.checkIn", inMenu) { sheet = .checkIn }.disabled(!enabled)
            case let .resumeRework(enabled):
                button(tr("Resume rework"), id: "job.resumeRework", inMenu) { Task { await model.resumeRework() } }
                    .disabled(!enabled)
            case .report:
                reportLink(inMenu)
            case .pause:
                button(tr("Pause work"), id: "job.pause", inMenu) { Task { await model.setPaused(true) } }
            case .resume:
                button(tr("Resume work"), id: "job.resume", inMenu) { Task { await model.setPaused(false) } }
            }
        }
        if inMenu { button } else { button.primaryAction() }
    }

    /// A step's button; in the bar its label takes the bar's width (a bordered button is as wide as its label).
    private func button(_ title: String, id: String, _ inMenu: Bool, action: @escaping () -> Void) -> some View {
        Button(action: action) {
            Text(verbatim: title).frame(maxWidth: inMenu ? nil : .infinity, minHeight: inMenu ? nil : Size.minTap)
        }
        .accessibilityIdentifier(id)
    }

    @ViewBuilder
    private func reportLink(_ inMenu: Bool) -> some View {
        if let report = model.report() {
            NavigationLink {
                ReportView(model: report)
            } label: {
                Label(tr("Open the work report"), symbol: .report)
                    .frame(maxWidth: inMenu ? nil : .infinity, minHeight: inMenu ? nil : Size.minTap)
            }
            .accessibilityIdentifier("job.report")
        }
    }

    private func bannerContent(_ banner: JobDetailModel.Banner) -> some View {
        StatusLabel(banner.text, tone: banner.tone).accessibilityIdentifier("job.banner")
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
