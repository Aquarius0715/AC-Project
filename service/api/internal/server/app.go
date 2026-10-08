// Package app is the composition root: configuration → pools → modules → Echo (backend Go design §2).
package server

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/pradita/ac-project/service/api/internal/modules/assets"
	"github.com/pradita/ac-project/service/api/internal/modules/audit"
	"github.com/pradita/ac-project/service/api/internal/modules/billing"
	"github.com/pradita/ac-project/service/api/internal/modules/control"
	"github.com/pradita/ac-project/service/api/internal/modules/devices"
	"github.com/pradita/ac-project/service/api/internal/modules/energy"
	"github.com/pradita/ac-project/service/api/internal/modules/identity"
	"github.com/pradita/ac-project/service/api/internal/modules/maintenance"
	"github.com/pradita/ac-project/service/api/internal/modules/monitoring"
	"github.com/pradita/ac-project/service/api/internal/modules/notify"
	"github.com/pradita/ac-project/service/api/internal/modules/restrictions"
	"github.com/pradita/ac-project/service/api/internal/modules/summaries"
	"github.com/pradita/ac-project/service/api/internal/modules/voice"
	"github.com/pradita/ac-project/service/api/internal/modules/writes"
	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/auth"
	"github.com/pradita/ac-project/service/api/internal/platform/blob"
	"github.com/pradita/ac-project/service/api/internal/platform/db"
	"github.com/pradita/ac-project/service/api/internal/platform/democlock"
)

// Config is the runtime configuration (container design §4 environment variables).
type Config struct {
	DatabaseURL       string
	DatabaseReaderURL string
	Addr              string
	Clock             func() time.Time
	DemoOps           bool         // demo environment only: demo.advanceClock / demo.trigger (IR154)
	DemoStart         time.Time    // demo scenario clock at start (fixture.clock, IR36); zero keeps the base clock
	Logger            *slog.Logger // Echo's application logger (Echo v5 uses log/slog); nil keeps Echo's default
}

// Server is the assembled Core API.
type Server struct {
	Echo     *echo.Echo
	Registry *ops.Registry
	DB       *db.TxManager
}

// New wires the registry, database and modules. The verifier is injected so tests can use static tokens.
func New(ctx context.Context, cfg Config, v auth.Verifier) (*Server, error) {
	m, err := db.Open(ctx, cfg.DatabaseURL, cfg.DatabaseReaderURL)
	if err != nil {
		return nil, err
	}
	reg := ops.NewRegistry()
	reg.DB, reg.Rec, reg.Idem = m, notifyingRecorder{inner: db.Recorder{}}, db.Idempotency{M: m}
	if cfg.Clock != nil {
		reg.Clock = cfg.Clock
	}
	var dc scenarioClock = &demoClock{base: reg.Clock}
	if cfg.DemoOps {
		if !cfg.DemoStart.IsZero() { // starts at fixture.clock, runs in real time, shared with the workers (IR36, IR168)
			shared, err := democlock.Open(ctx, m.Writer, reg.Clock, cfg.DemoStart)
			if err != nil {
				m.Close()
				return nil, err
			}
			dc = shared
		}
		reg.Clock = dc.Now
	}
	registerDemo(reg, &demoOps{enabled: cfg.DemoOps, m: m, clock: dc})
	identity.Register(reg)
	identity.RegisterMembers(reg)
	am := &assets.Module{Mon: monitorReads{alerts: monitoring.Alerts{}, telemetry: monitoring.Telemetry{Sensors: devices.Models{}}}, Models: devices.Models{}, Policies: monitoring.Policies{},
		Usage:     []assets.UnitUsage{billing.Usage{}, maintenance.Usage{}, devices.Usage{}},
		Contracts: billing.Usage{}, Claims: maintenance.Usage{}, Import: devices.Models{},
		Activity: []assets.UnitUsage{monitoring.Telemetry{}, maintenance.Usage{}}, Store: m,
		Active:    []assets.UnitActivity{maintenance.Usage{}, devices.Usage{}, restrictions.Busy{}, control.Busy{}, billing.Usage{}},
		OrgActive: []assets.OrgActivity{billing.Usage{}, maintenance.Usage{}}}
	am.Details = unitDetails{}
	assets.Register(reg, am)
	devices.Register(reg, devices.Capabilities{Units: am})
	monitoring.RegisterAlerts(reg, monitoring.Alerts{Units: am, Access: maintenance.Access{}})
	jobs := maintenance.Jobs{Units: am, Severity: monitoring.Alerts{}, HQOrg: func(c *ops.Call) uuid.UUID { return c.Principal.OrgID }, Sites: am}
	maintenance.RegisterJobs(reg, jobs)
	maintenance.RegisterOnSite(reg, maintenance.OnSite{Jobs: jobs})
	blobs := blob.NewDirFromEnv()
	reports := maintenance.Reports{Jobs: jobs, Units: am}
	maintenance.RegisterFollowUps(reg, maintenance.FollowUps{Jobs: jobs, Blobs: blobs, Warranty: am})
	maintenance.RegisterReports(reg, reports)
	maintenance.RegisterPlans(reg, maintenance.Plans{Jobs: jobs})
	maintenance.RegisterPartners(reg, maintenance.Partners{Orgs: identity.Directory{}, Customers: am})
	maintenance.RegisterFiles(reg, maintenance.Files{Reports: reports, Blobs: blobs})
	delivery := maintenance.Delivery{Jobs: jobs, Directory: identity.Directory{}, Units: am}
	maintenance.RegisterDelivery(reg, delivery)
	maintenance.RegisterProposals(reg, maintenance.Proposals{Delivery: delivery})
	cmds := control.Commands{Units: controlTargets{am}, Devices: devices.Models{}, Restrictions: restrictions.Busy{}, Access: maintenance.Access{}}
	control.Register(reg, cmds)
	control.RegisterAutomations(reg, control.Automations{Units: controlTargets{am}, Cmd: cmds, Notifier: monitoring.Notifier{}})
	control.RegisterDiagnostics(reg, control.Diagnostics{Commands: cmds, Jobs: maintenance.Access{}})
	identity.RegisterPreferences(reg)
	identity.RegisterClientUsers(reg)
	identity.RegisterTwoFactor(reg)
	writes.Register(reg)
	maintenance.RegisterQr(reg)
	energy.Register(reg)
	voice.Register(reg, voice.Voice{Units: am})
	energy.RegisterMRV(reg)
	energy.RegisterOffsets(reg)
	sums := summaries.Summaries{Units: am, Jobs: jobs}
	summaries.Register(reg, sums)
	summaries.RegisterAdmin(reg, sums)
	notify.Register(reg)
	rm := restrictions.Restrictions{Units: restrictionTargets{am}, Devices: devices.Models{}}
	restrictions.Register(reg, rm)
	bill := billing.Billing{Customers: am, Restrictions: restrictions.Busy{}, Recipients: identity.Directory{}, Releases: rm}
	billing.Register(reg, bill)
	billing.RegisterPayments(reg, bill)
	billing.RegisterInquiries(reg, bill)
	audit.Register(reg)
	billing.RegisterPayouts(reg, billing.Payouts{Source: maintenance.PayoutSource{Jobs: jobs}})
	maintenance.RegisterWorkforce(reg, maintenance.Workforce{Delivery: delivery})
	maintenance.RegisterFilterCare(reg, maintenance.FilterCare{Units: am, Run: monitoring.Telemetry{Sensors: devices.Models{}}})
	maintenance.RegisterCertificates(reg, maintenance.Certificates{Delivery: delivery, Grants: identity.Directory{}, Blobs: blobs})
	monitoring.RegisterTelemetry(reg, monitoring.Telemetry{Units: am, Sensors: devices.Models{}})
	monitoring.RegisterPolicies(reg, monitoring.Policies{Units: am, Recipients: identity.Directory{}, Caps: unitCaps{am}})
	dm := &devices.Module{Units: am, Exclusion: []devices.Exclusion{control.Busy{}, restrictions.Busy{}}}
	devices.RegisterDevices(reg, dm)
	devices.RegisterCampaigns(reg, dm)

	e := newEcho(reg, m, &auth.Authenticator{Verifier: v, DB: m, Now: reg.Clock}, cfg.Logger)
	return &Server{Echo: e, Registry: reg, DB: m}, nil
}

// monitorReads combines the Monitoring read interfaces Assets uses.
type monitorReads struct {
	alerts    monitoring.Alerts
	telemetry monitoring.Telemetry
}

func (m monitorReads) PowerMeasured(ctx context.Context, c *ops.Call, units []uuid.UUID) (map[uuid.UUID]bool, error) {
	return m.telemetry.PowerMeasured(ctx, c, units)
}

func (m monitorReads) ActiveAlertCounts(ctx context.Context, c *ops.Call, units []uuid.UUID) (map[uuid.UUID]int, error) {
	return m.alerts.ActiveAlertCounts(ctx, c, units)
}

// unitCaps joins the unit's model version (Assets) with its Capability (Devices) for UnitAction checks.
type unitCaps struct{ am *assets.Module }

func (u unitCaps) UnitCaps(ctx context.Context, c *ops.Call, unit uuid.UUID) (control.Caps, bool, error) {
	info, ok, err := u.am.UnitInfo(ctx, c, unit)
	if err != nil || !ok {
		return control.Caps{}, false, err
	}
	return devices.Models{}.Caps(ctx, c, info.ModelID, info.CapabilityVersion)
}

// restrictionTargets joins Assets (organization, archived) with the unit's capability for Restrictions.
type restrictionTargets struct{ am *assets.Module }

func (t restrictionTargets) RestrictionTarget(ctx context.Context, c *ops.Call, unit uuid.UUID) (restrictions.UnitCaps, bool, error) {
	ct, ok, err := controlTargets{t.am}.Target(ctx, c, unit)
	if err != nil || !ok {
		return restrictions.UnitCaps{}, false, err
	}
	org, archived, _, err := t.am.UnitState(ctx, c, unit)
	if err != nil {
		return restrictions.UnitCaps{}, false, err
	}
	return restrictions.UnitCaps{OrgID: org, Archived: archived, Control: ct.Caps.Control, HasTemperature: ct.Caps.HasTemperature,
		TempMin: ct.Caps.TempMin, TempMax: ct.Caps.TempMax, Step: ct.Caps.TempStep}, true, nil
}

// controlTargets joins Assets (organization, property, version) with the unit's capability for Control.
type controlTargets struct{ am *assets.Module }

func (t controlTargets) Target(ctx context.Context, c *ops.Call, unit uuid.UUID) (control.Target, bool, error) {
	info, ok, err := t.am.UnitInfo(ctx, c, unit)
	if err != nil || !ok {
		return control.Target{}, false, err
	}
	v, err := t.am.UnitVersion(ctx, c, unit)
	if err != nil {
		return control.Target{}, false, err
	}
	caps, _, err := devices.Models{}.Caps(ctx, c, info.ModelID, info.CapabilityVersion)
	return control.Target{OrgID: info.OrgID, PropertyID: info.PropertyID, Version: v, Caps: caps}, true, err
}
