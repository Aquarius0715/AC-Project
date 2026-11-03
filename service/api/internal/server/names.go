package server

// The values the server compares and writes by name: audiences, permissions, kinds, events, notice types and
// measurement qualities.
const (
	audienceClients             = "clients"
	audienceHQ                  = "hq"
	audienceTechnician          = "technician"
	permPartnerAccept           = "partner.accept"
	permPartnerAssign           = "partner.assign"
	permPartnerReview           = "partner.review"
	eventCommandAck             = "command_ack"
	eventCommandFail            = "command_fail"
	eventCommandSent            = "command_sent"
	eventOffered                = "offered"
	eventRestrictionObservation = "restriction_observation"
	keyState                    = "state"
	kindRestored                = "restored"
	noticeJobUpdate             = "job_update"
	qualitySuspect              = "suspect"
	statusCompleted             = "completed"
)
