import SwiftUI

/// The notifications of both apps (FR-X07): the unread ones marked, a tap marks one read, all of them at once with
/// "Mark all as read". Reading a notification does not resolve the alert it is about. Cards in a scroll view, as every
/// screen that may be long (CardView.swift).
public struct NotificationsView: View {
    @State private var model: NotificationsModel

    public init(model: NotificationsModel) {
        _model = State(initialValue: model)
    }

    public var body: some View {
        NavigationStack {
            Group {
                Card {
                    HStack {
                        filterButton(tr("All"), .all)
                        filterButton(tr("Unread"), .unread)
                    }
                    if model.unreadCount > 0 {
                        Button(tr("Mark all as read")) { Task { await model.markAllRead() } }
                            .secondaryAction()
                            .accessibilityIdentifier("notifications.markAll")
                    }
                    if let message = model.notifications.message ?? model.note {
                        Text(verbatim: message).accessibilityIdentifier("notifications.message")
                    }
                }
                ForEach(model.rows) { row in
                    Button {
                        Task { await model.markRead(row) }
                    } label: {
                        Card { label(row) }
                    }
                    .buttonStyle(.plain)
                    .accessibilityIdentifier("notification.\(row.id)")
                }
                if let empty = model.emptyText {
                    Card { Text(verbatim: empty) }
                }
            }
            .cardScreen()
            .overlay { if case .loading = model.notifications { ProgressView() } }
            .navigationTitle(Text(verbatim: tr("Notifications")))
            .inlineTitle()
            .refreshable { await model.load() }
            .task { await model.load() }
        }
    }

    @ViewBuilder
    private func filterButton(_ title: String, _ filter: NotificationsModel.Filter) -> some View {
        if model.filter == filter {
            Button(title) { model.filter = filter }.buttonStyle(.borderedProminent).accessibilityAddTraits(.isSelected)
        } else {
            Button(title) { model.filter = filter }.secondaryAction()
        }
    }

    private func label(_ row: NotificationsModel.Row) -> some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack(alignment: .firstTextBaseline, spacing: 6) {
                if !row.isRead {
                    Image(systemName: "circle.fill").foregroundStyle(Brand.primary).accessibilityLabel(Text(verbatim: tr("Unread")))
                }
                Text(verbatim: row.title).font(row.isRead ? .body : .headline)
            }
            if !row.detail.isEmpty { Text(verbatim: row.detail).font(.subheadline) }
            Text(verbatim: row.when).font(.footnote)
        }
        .foregroundStyle(.primary)
    }
}
