import ACKit
import ACShared
import Charts
import SwiftUI

/// The technician's unit screen: the register and the components, or the monitoring (latest values, a chart of the
/// period, the other metrics and the period's events), with the open alerts and the technician's jobs on the unit.
struct TechUnitView: View {
    @State private var model: TechUnitModel
    @State private var tab = Tab.register

    enum Tab: Hashable { case register, monitoring }

    init(model: TechUnitModel) {
        _model = State(initialValue: model)
    }

    var body: some View {
        Group {
            if let message = model.content.message {
                Card { Text(verbatim: message).accessibilityIdentifier("unit.message") }
            }
            switch model.content.value {
            case .unit?: unitCards
            case let .notStarted(text)?:
                Card(tr("Not started yet")) { Text(verbatim: text).accessibilityIdentifier("unit.notStarted") }
            case .notFound?:
                Card(tr("This page isn’t available")) {
                    Text(verbatim: tr("The page doesn’t exist, or your account can’t open it. Nothing was navigated automatically."))
                }
            case nil: ProgressView()
            }
        }
        .cardScreen()
        .navigationTitle(Text(verbatim: model.unit?.displayName ?? tr("Unit")))
        .inlineTitle()
        .hideTabBar()
        .task { await model.load() }
        .refreshable { await model.load() }
    }

    @ViewBuilder private var unitCards: some View {
        if let stopped = model.stopped {
            Card {
                Label {
                    Text(verbatim: stopped)
                } icon: {
                    Image(systemName: "wifi.slash").foregroundStyle(.red)
                }
            }
        }
        Card {
            HStack {
                choice(.register, tr("Register"), id: "unit.tab.register")
                choice(.monitoring, tr("Monitoring"), id: "unit.tab.monitoring")
            }
        }
        if tab == .register {
            Card(tr("Unit register")) {
                Facts(rows: model.register)
                Text(verbatim: tr("Missing values are never filled from similar models or today’s date.")).font(.footnote)
            }
            Card(tr("Components ({n})", ["n": String(model.unit?.components.count ?? 0)])) {
                if model.components.isEmpty { Text(verbatim: tr("No components in this unit’s service scope.")) }
                Facts(rows: model.components)
                Text(verbatim: tr("Each component gets its result in the job workspace — normal is never preselected.")).font(.footnote)
            }
        } else {
            tilesCard
            chartCard
            Card(tr("Events in this period")) {
                rows(model.periodEvents, empty: tr("No alert or device events in this period."), id: "unit.events")
            }
        }
        Card(tr("Open alerts ({n})", ["n": String(model.openAlerts.count)])) {
            if model.openAlerts.isEmpty { Text(verbatim: tr("No open alerts.")).accessibilityIdentifier("unit.alerts.empty") }
            ForEach(model.openAlerts) { row in
                NavigationLink {
                    AlertsView(model: AlertsModel(unitId: model.unitId, jobId: model.jobId, alertId: row.id, client: model.client))
                } label: {
                    VStack(alignment: .leading, spacing: 2) {
                        Text(verbatim: row.title).font(.headline)
                        Text(verbatim: row.badge).font(.subheadline.weight(.semibold))
                        Text(verbatim: row.sub).font(.subheadline)
                    }
                    .multilineTextAlignment(.leading)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .foregroundStyle(.primary)
                }
                .accessibilityIdentifier("unit.alerts.\(row.id)")
            }
        }
        Card(tr("Maintenance history")) {
            rows(model.history, empty: tr("No jobs of yours on this unit."), id: "unit.history")
            Text(verbatim: tr("Only your own assignments on this unit are listed.")).font(.footnote)
        }
    }

    private func choice(_ value: Tab, _ title: String, id: String) -> some View {
        Group {
            if tab == value {
                Button(title) { tab = value }.buttonStyle(.borderedProminent)
            } else {
                Button(title) { tab = value }.secondaryAction()
            }
        }
        .accessibilityIdentifier(id)
        .accessibilityAddTraits(tab == value ? .isSelected : [])
    }

    @ViewBuilder private var tilesCard: some View {
        Card(tr("Time-series monitoring")) {
            ForEach(Array(model.tiles.enumerated()), id: \.offset) { _, tile in
                VStack(alignment: .leading, spacing: 2) {
                    Text(verbatim: tile.label).font(.subheadline.weight(.semibold))
                    Text(verbatim: tile.value).font(.title3)
                    Text(verbatim: tile.sub).font(.footnote)
                }
                .frame(maxWidth: .infinity, alignment: .leading)
                .accessibilityElement(children: .combine)
            }
        }
    }

    @ViewBuilder private var chartCard: some View {
        Card(model.chartTitle) {
            HStack {
                ForEach(SeriesPeriod.allCases, id: \.self) { period in
                    if model.period == period {
                        Button(period.label) { Task { await model.show(period) } }.buttonStyle(.borderedProminent)
                    } else {
                        Button(period.label) { Task { await model.show(period) } }.secondaryAction()
                    }
                }
            }
            .accessibilityIdentifier("unit.periods")
            if model.lines.allSatisfy(\.points.isEmpty) {
                Text(verbatim: tr("No measurements yet")).accessibilityIdentifier("unit.chart.empty")
            } else {
                Chart {
                    ForEach(model.lines, id: \.metric) { line in
                        ForEach(line.points) { point in
                            // each stretch its own series: the line does not cross a gap
                            LineMark(
                                x: .value(tr("Time"), point.at),
                                y: .value(line.name, point.value),
                                series: .value(tr("Metric"), "\(line.name) \(point.segment)")
                            )
                            .foregroundStyle(by: .value(tr("Metric"), line.name))
                        }
                    }
                }
                // the lines are named below in the body colour: the chart's own legend is grey, below the audit's contrast
                .chartLegend(.hidden)
                .frame(height: 200)
                // one element that says what it shows: the axis labels the chart draws are no elements of their own
                .accessibilityElement(children: .ignore)
                .accessibilityLabel(Text(verbatim: model.chartTitle))
                .accessibilityValue(Text(verbatim: model.lines.compactMap { $0.count ?? $0.cut }.joined(separator: " · ")))
                .accessibilityIdentifier("unit.chart")
            }
            ForEach(model.lines, id: \.metric) { line in
                if let text = line.count ?? line.cut { Text(verbatim: "\(line.name): \(text)").font(.footnote) }
                if let gap = line.gap { Text(verbatim: gap).font(.footnote) }
            }
            metricChoices
        }
    }

    @ViewBuilder private var metricChoices: some View {
        ForEach(model.measured, id: \.self) { metric in
            Toggle(
                isOn: Binding(get: { model.metrics.contains(metric) }, set: { _ in Task { await model.toggle(metric) } })
            ) { Text(verbatim: UnitWords.metric(metric)) }
            .disabled(!model.canAdd(metric))
            .accessibilityIdentifier("unit.metric.\(metric.rawValue)")
        }
    }

    @ViewBuilder
    private func rows(_ rows: [UnitRow], empty: String, id: String) -> some View {
        if rows.isEmpty { Text(verbatim: empty).accessibilityIdentifier("\(id).empty") }
        ForEach(rows) { row in
            VStack(alignment: .leading, spacing: 2) {
                Text(verbatim: row.title).font(.headline)
                Text(verbatim: row.badge).font(.subheadline.weight(.semibold))
                if !row.sub.isEmpty { Text(verbatim: row.sub).font(.subheadline) }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .accessibilityElement(children: .combine)
            .accessibilityIdentifier("\(id).\(row.id)")
        }
    }
}
