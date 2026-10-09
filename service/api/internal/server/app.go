// Package app is the composition root: configuration → pools → modules → Echo (backend Go design §2).
package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"time"

	"github.com/pradita/ac-project/service/api/internal/platform/events"

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
	DemoOps           bool              // demo environment only: demo.advanceClock / demo.trigger (IR154)
	DemoStart         time.Time         // demo scenario clock at start (fixture.clock, IR36); zero keeps the base clock
	Logger            *slog.Logger      // Echo's application logger (Echo v5 uses log/slog); nil keeps Echo's default
	Domains           []string          // business domains this service serves (IR180); empty = all (tests)
	IdentityURL       string            // identity-api base URL for principals (IR181); empty = read identity tables
	InternalToken     string            // shared token of internal service-to-service endpoints
	ServiceURLs       map[string]string // base URL per domain for internal queries of unserved domains (IR190)
	PrincipalTTL      time.Duration     // principal cache of RemoteSource: 0 = 30 s, negative = no cache (IR181)
	DemoClock         ScenarioClock     // shared scenario clock of several services in one binary (tests); overrides DemoStart
}

// Server is the assembled Core API.
type Server struct {
	Echo      *echo.Echo
	Registry  *ops.Registry
	DB        *db.TxManager
	Consumers []*events.Consumer // event subscribers of the served domains (IR183)
}

// DrainEvents applies every pending event of this server's consumers (inline mode and tests).
func (s *Server) DrainEvents(ctx context.Context) error {
	for pass := 0; pass < 10; pass++ { // events can trigger further events: repeat until nothing is pending
		applied := 0
		for _, c := range s.Consumers {
			n, err := c.Drain(ctx)
			if err != nil {
				return err
			}
			applied += n
		}
		if applied == 0 {
			return nil
		}
	}
	return nil
}

// New wires the registry, database and modules. The verifier is injected so tests can use static tokens.
func New(ctx context.Context, cfg Config, v auth.Verifier) (*Server, error) {
	m, err := db.Open(ctx, cfg.DatabaseURL, cfg.DatabaseReaderURL)
	if err != nil {
		return nil, err
	}
	if err := ops.CheckDomains(); err != nil {
		return nil, err
	}
	reg := ops.NewRegistry()
	reg.ServeDomains(cfg.Domains...)
	reg.DB, reg.Rec, reg.Idem = m, notifyingRecorder{inner: db.Recorder{}}, db.Idempotency{M: m}
	if cfg.Clock != nil {
		reg.Clock = cfg.Clock
	}
	var dc ScenarioClock = &demoClock{base: reg.Clock}
	if cfg.DemoOps {
		if cfg.DemoClock != nil {
			dc = cfg.DemoClock
		} else if !cfg.DemoStart.IsZero() { // starts at fixture.clock, runs in real time, shared with the workers (IR36, IR168)
			shared, err := democlock.Open(ctx, m.Writer, reg.Clock, cfg.DemoStart)
			if err != nil {
				m.Close()
				return nil, err
			}
			dc = shared
		}
		reg.Clock = dc.Now
	}
	registerDemo(reg, &demoOps{inline: len(cfg.Domains) == 0, enabled: cfg.DemoOps, m: m, clock: dc, reg: reg})
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
	eq := equipmentView{am: am, alerts: monitoring.Alerts{}, tel: monitoring.Telemetry{Sensors: devices.Models{}}, models: devices.Models{}} // other domains' view (IR193)
	devices.Register(reg, devices.Capabilities{Units: am})
	monitoring.RegisterAlerts(reg, monitoring.Alerts{Units: am, Access: maintenance.Access{}})
	jobs := maintenance.Jobs{Units: eq, Severity: eq, HQOrg: func(c *ops.Call) uuid.UUID { return c.Principal.OrgID }, Sites: eq}
	maintenance.RegisterJobs(reg, jobs)
	maintenance.RegisterOnSite(reg, maintenance.OnSite{Jobs: jobs})
	blobs := blob.NewDirFromEnv()
	reports := maintenance.Reports{Jobs: jobs, Units: eq}
	maintenance.RegisterFollowUps(reg, maintenance.FollowUps{Jobs: jobs, Blobs: blobs, Warranty: eq})
	maintenance.RegisterReports(reg, reports)
	maintenance.RegisterPlans(reg, maintenance.Plans{Jobs: jobs})
	maintenance.RegisterPartners(reg, maintenance.Partners{Orgs: identity.Directory{}, Customers: eq})
	maintenance.RegisterFiles(reg, maintenance.Files{Reports: reports, Blobs: blobs})
	delivery := maintenance.Delivery{Jobs: jobs, Directory: identity.Directory{}, Units: eq}
	maintenance.RegisterDelivery(reg, delivery)
	maintenance.RegisterProposals(reg, maintenance.Proposals{Delivery: delivery})
	cmds := control.Commands{Units: controlTargets{am}, Devices: devices.Models{}, Restrictions: restrictions.Busy{}, Access: maintenance.Access{}}
	control.Register(reg, cmds)
	automations := control.Automations{Units: controlTargets{am}, Cmd: cmds, Notifier: monitoring.Notifier{}}
	control.RegisterAutomations(reg, automations)
	reg.AddJob(ops.DomainEquipment, automations.FireSchedules) // IR54: schedule occurrences fire from the equipment scheduler
	control.RegisterDiagnostics(reg, control.Diagnostics{Commands: cmds, Jobs: maintenance.Access{}})
	identity.RegisterPreferences(reg)
	identity.RegisterClientUsers(reg)
	identity.RegisterTwoFactor(reg)
	writes.Register(reg)
	assets.RegisterQr(reg, maintenance.QrAccess{}) // an Assets operation; maintenance answers the assignment (IR194)
	energy.Register(reg)
	voice.Register(reg, voice.Voice{Units: am})
	energy.RegisterMRV(reg)
	energy.RegisterOffsets(reg)
	sums := summaries.Summaries{Units: am, Registry: reg}
	maintenance.RegisterQueries(reg, jobs) // internal queries of the read models (IR190)
	billing.RegisterQueries(reg)
	energy.RegisterQueries(reg)
	identity.RegisterQueries(reg)
	restrictions.RegisterQueries(reg)
	audit.RegisterQueries(reg)
	summaries.Register(reg, sums)
	summaries.RegisterAdmin(reg, sums)
	notify.Register(reg)
	rm := restrictions.Restrictions{Units: eq, Devices: eq}
	restrictions.Register(reg, rm)
	bill := billing.Billing{Customers: eq, Restrictions: restrictions.Busy{}, Recipients: identity.Directory{}, Releases: rm}
	billing.Register(reg, bill)
	billing.RegisterPayments(reg, bill)
	billing.RegisterInquiries(reg, bill)
	audit.Register(reg)
	mv := maintenanceView{src: maintenance.PayoutSource{Jobs: jobs}}
	billing.RegisterPayouts(reg, billing.Payouts{Source: mv})
	registerCrossDomain(reg, eq, mv)
	maintenance.RegisterWorkforce(reg, maintenance.Workforce{Delivery: delivery})
	maintenance.RegisterFilterCare(reg, maintenance.FilterCare{Units: eq, Run: eq})
	maintenance.RegisterCertificates(reg, maintenance.Certificates{Delivery: delivery, Grants: identity.Directory{}, Blobs: blobs})
	monitoring.RegisterTelemetry(reg, monitoring.Telemetry{Units: am, Sensors: devices.Models{}})
	monitoring.RegisterPolicies(reg, monitoring.Policies{Units: am, Recipients: identity.Directory{}, Caps: unitCaps{am}})
	dm := &devices.Module{Units: am, Exclusion: []devices.Exclusion{control.Busy{}, restrictions.Busy{}}}
	devices.RegisterDevices(reg, dm)
	devices.RegisterCampaigns(reg, dm)
	reg.AddJob(ops.DomainEquipment, devices.Lifecycle{Guard: control.Busy{}}.Advance) // IR67: operations start and time out on the equipment scheduler

	authn := &auth.Authenticator{Verifier: v, DB: m, Now: reg.Clock}
	servesIdentity := len(cfg.Domains) == 0 || slices.Contains(cfg.Domains, ops.DomainIdentity)
	if cfg.IdentityURL != "" && !servesIdentity { // IR181 step 1: principals come from identity-api
		ttl := cfg.PrincipalTTL
		if ttl == 0 {
			ttl = 30 * time.Second
		}
		authn.Source = &auth.RemoteSource{BaseURL: cfg.IdentityURL, Token: cfg.InternalToken, TTL: ttl}
	}
	e, err := newEcho(reg, m, authn, cfg.Logger, cfg.InternalToken, servesIdentity)
	if err != nil {
		m.Close()
		return nil, err
	}
	srv := &Server{Echo: e, Registry: reg, DB: m, Consumers: consumers(m, cfg.Domains)}
	if len(cfg.Domains) > 0 && len(cfg.ServiceURLs) > 0 { // read models ask the owning services (IR190)
		q := &ops.HTTPQueries{Targets: map[string]*url.URL{}, Token: cfg.InternalToken}
		for d, raw := range cfg.ServiceURLs {
			u, err := url.Parse(raw)
			if err != nil || u.Scheme == "" || u.Host == "" {
				return nil, fmt.Errorf("server: service URL of %s is not absolute", d)
			}
			q.Targets[d] = u
		}
		reg.Remote = q
	}
	if len(cfg.Domains) == 0 { // one process serves every domain: apply events inline after each write
		reg.AfterCommit = func(ctx context.Context) {
			if err := srv.DrainEvents(context.WithoutCancel(ctx)); err != nil && cfg.Logger != nil {
				cfg.Logger.Error("inline events", "error", err)
			}
		}
		reg.BeforeDispatch = reg.AfterCommit
	}
	return srv, nil
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

func (m monitorReads) LatestMeasurements(ctx context.Context, c *ops.Call, units []uuid.UUID) (map[uuid.UUID][]any, error) {
	latest, err := m.telemetry.LatestMeasurements(ctx, c, units)
	out := make(map[uuid.UUID][]any, len(latest))
	for u, ms := range latest {
		for _, x := range ms {
			out[u] = append(out[u], x)
		}
	}
	return out, err
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
