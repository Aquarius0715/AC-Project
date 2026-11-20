import ACKit

/// The words of the contract's states, as the web apps word them (shared/lib/jobs.ts and the unit pages): one switch
/// per state, so every text is a literal the generator finds and a new state does not compile without its words.
public enum Labels {
    public static func job(_ status: JobDisplayStatus) -> String {
        switch status {
        case .requested: tr("Requested")
        case .timeProposed: tr("Time proposed")
        case .offered: tr("Offered")
        case .accepted: tr("Accepted")
        case .assigned: tr("Assigned")
        case .inProgress: tr("In progress")
        case .submitted: tr("Submitted")
        case .reworkRequested: tr("Rework requested")
        case .completed: tr("Completed")
        case .cancelled: tr("Cancelled")
        case .onHold: tr("On hold")
        }
    }

    /// A job's own status: the display statuses without time_proposed, which only the display status says.
    public static func job(_ status: JobStatus) -> String {
        switch status {
        case .requested: job(JobDisplayStatus.requested)
        case .offered: job(JobDisplayStatus.offered)
        case .accepted: job(JobDisplayStatus.accepted)
        case .assigned: job(JobDisplayStatus.assigned)
        case .inProgress: job(JobDisplayStatus.inProgress)
        case .onHold: job(JobDisplayStatus.onHold)
        case .submitted: job(JobDisplayStatus.submitted)
        case .reworkRequested: job(JobDisplayStatus.reworkRequested)
        case .completed: job(JobDisplayStatus.completed)
        case .cancelled: job(JobDisplayStatus.cancelled)
        }
    }

    public static func jobType(_ type: MaintenanceJobType) -> String {
        switch type {
        case .periodic: tr("Periodic inspection")
        case .reactive: tr("Repair")
        case .preventive: tr("Preventive maintenance")
        case .installation: tr("Installation")
        }
    }

    public static func power(_ state: EffectivePowerState) -> String {
        switch state {
        case .on: tr("On")
        case .off: tr("Off")
        case .unknown: tr("Power unknown")
        }
    }

    public static func connection(_ connection: Connection) -> String {
        switch connection {
        case .online: tr("Online")
        case .offline: tr("Offline")
        case .connecting: tr("Connecting")
        case .error: tr("Error")
        case .unknown: tr("Connection unknown")
        }
    }
}
