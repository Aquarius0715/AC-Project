import ACKit
import Observation

/// The signed-in membership's notifications (notifications.list, FR-X07): newest first, unread ones marked; reading one
/// marks it read with its version, and "Mark all as read" marks the listed unread ones in one write (IR379).
@MainActor @Observable
public final class NotificationsModel {
    /// One notification as its row shows it.
    public struct Row: Identifiable, Equatable, Sendable {
        public let id: String
        public let version: Double
        public let title: String
        public let detail: String
        public let when: String
        public let isRead: Bool
    }

    public enum Filter: Sendable { case all, unread }

    public private(set) var notifications: Loadable<[Row]> = .loading
    /// What the last mark came to, when it says something the list does not.
    public private(set) var note: String?
    public var filter: Filter = .all
    let client: CoreClient

    public init(client: CoreClient) {
        self.client = client
    }

    public func load() async {
        notifications = await notifications.after { () async throws(CoreError) in
            let query = Query(limit: 100, sort: QuerySort(field: .occurredAt, direction: .desc))
            return try await client.call(Ops.NotificationsList.self, query).data.items.map(Self.row)
        }
    }

    /// The rows of the filter.
    public var rows: [Row] {
        let all = notifications.value ?? []
        return filter == .all ? all : all.filter { !$0.isRead }
    }

    public var unreadCount: Int { (notifications.value ?? []).filter { !$0.isRead }.count }

    /// Marks one read (opening it); one that changed meanwhile shows its latest state.
    public func markRead(_ row: Row) async {
        guard !row.isRead else { return }
        do {
            _ = try await client.call(
                Ops.NotificationsMarkRead.self,
                NotificationsMarkReadInput(id: row.id),
                expectedVersion: Int(row.version)
            )
            note = nil
        } catch {
            note = Self.markRefusal(error)
        }
        await load()
    }

    /// Marks every listed unread notification read in one write.
    public func markAllRead() async {
        let ids = (notifications.value ?? []).filter { !$0.isRead }.map(\.id)
        guard !ids.isEmpty else { return }
        do {
            let marked = try await client.call(Ops.NotificationsMarkAllRead.self, NotificationsMarkAllReadInput(ids: ids)).data
            let count = marked.markedIds.count
            note = count == 1 ? tr("1 notification marked as read") : tr("{n} notifications marked as read", ["n": String(count)])
        } catch {
            note = ErrorText.of(error)
        }
        await load()
    }

    static func markRefusal(_ error: CoreError) -> String {
        switch error.error.code {
        case .conflict: tr("This notification changed — the latest state is shown.")
        case .notFound: tr("This notification is no longer available.")
        default: ErrorText.of(error)
        }
    }

    static func row(_ notification: CoreNotification) -> Row {
        Row(
            id: notification.id,
            version: notification.version,
            title: NotificationText.title(notification),
            detail: NotificationText.detail(notification),
            when: Format.dateTime(notification.occurredAt),
            isRead: notification.readAt != nil
        )
    }
}
