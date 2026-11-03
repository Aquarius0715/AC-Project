// Package server is the composition root: configuration → pools → modules → Echo (backend Go design §2).
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/auth"
	"github.com/pradita/ac-project/service/api/internal/platform/blob"
	"github.com/pradita/ac-project/service/api/internal/platform/db"
	"github.com/pradita/ac-project/service/api/internal/platform/democlock"
	"github.com/pradita/ac-project/service/api/internal/platform/events"
	"github.com/pradita/ac-project/service/api/internal/platform/ratelimit"
)

// errRelativeURL refuses a domain service URL without a scheme and a host.
var errRelativeURL = errors.New("is not absolute")

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
	// shared scenario clock of several services in one binary (tests); overrides DemoStart
	DemoClock  ScenarioClock
	WriteLimit ops.WriteLimiter // writes per membership and minute (IR375); nil = no limit
	// denied writes recorded per actor, operation and minute (IR603); nil = this process counts
	DenyLimit ops.WriteLimiter
	// ends sessions (IR386) and opens / turns off accounts (IR461) at the identity provider; nil = none
	IdentityProvider identity.Provider
	AccountApps      identity.AccountApps // the apps a new account's set-up e-mail returns to (IR461)
	// accounts a contractor or customer owner adds per organization in 24 h (IR464); 0 = 30
	AccountsPerDay int
	// tests only: wraps each operation's transaction to inject database failures; nil = none
	WrapTx func(pgx.Tx) pgx.Tx
}

// Server is the assembled Core API.
type Server struct {
	Echo      *echo.Echo
	Registry  *ops.Registry
	DB        *db.TxManager
	Consumers []*events.Consumer // event subscribers of the served domains (IR183)
}

// New wires the registry, database and modules. The verifier is injected so tests can use static tokens.
func New(ctx context.Context, cfg Config, verifier auth.Verifier) (*Server, error) {
	txm, err := db.Open(ctx, cfg.DatabaseURL, cfg.DatabaseReaderURL)
	if err != nil {
		return nil, apperr.Wrap(err, "db.Open")
	}

	checkDomainsErr := ops.CheckDomains()
	if checkDomainsErr != nil {
		return nil, apperr.Wrap(checkDomainsErr, "ops.CheckDomains")
	}

	reg := newRegistry(cfg, txm)

	demo, err := newDemoOps(ctx, cfg, txm, reg)
	if err != nil {
		txm.Close()

		return nil, err
	}

	registerModules(reg, txm, demo, identity.Accounts{PerOrgPerDay: cfg.AccountsPerDay})

	servesIdentity := len(cfg.Domains) == 0 || slices.Contains(cfg.Domains, ops.DomainIdentity)

	//nolint:contextcheck // the handlers run on each request's own context, not on the one the server starts with
	app, err := newEcho(
		reg,
		txm,
		newAuthenticator(cfg, verifier, txm, reg.Clock, servesIdentity),
		cfg.Logger,
		cfg.InternalToken,
		servesIdentity,
	)
	if err != nil {
		txm.Close()

		return nil, err
	}

	srv := &Server{
		Echo:      app,
		Registry:  reg,
		DB:        txm,
		Consumers: consumers(txm, cfg.Domains, cfg.IdentityProvider, cfg.AccountApps),
	}

	err = useRemoteQueries(reg, cfg)
	if err != nil {
		return nil, err
	}

	reg.WrapTx = cfg.WrapTx
	if len(
		cfg.Domains,
	) == 0 { // one process serves every domain: apply events inline after each write
		reg.AfterCommit = srv.inlineEvents(cfg.Logger)
		reg.BeforeDispatch = reg.AfterCommit
	}

	return srv, nil
}

// newDemoOps is demo.* with its scenario clock: the configured one, the clock shared in the database when the scenario
// has a start, or one local to the process. In the demo the registry runs on that clock.
func newDemoOps(
	ctx context.Context,
	cfg Config,
	txm *db.TxManager,
	reg *ops.Registry,
) (*demoOps, error) {
	demo := &demoOps{
		inline:  len(cfg.Domains) == 0,
		enabled: cfg.DemoOps,
		m:       txm,
		clock:   NewScenarioClock(reg.Clock),
		reg:     reg,
		notify:  nil,
	}
	if !cfg.DemoOps {
		return demo, nil
	}

	if cfg.DemoClock != nil {
		demo.clock = cfg.DemoClock
	} else if !cfg.DemoStart.IsZero() {
		// starts at fixture.clock, runs in real time, shared with the workers (IR36, IR168)
		shared, err := democlock.Open(ctx, txm.Writer, reg.Clock, cfg.DemoStart)
		if err != nil {
			return nil, apperr.Wrap(err, "democlock.Open")
		}

		demo.clock = shared
	}

	reg.Clock = demo.clock.Now

	return demo, nil
}

// wiring is what the later registration steps share: the registry, the demo operations, the equipment and
// maintenance parts the first two steps made, and the configured account cap.
type wiring struct {
	reg       *ops.Registry
	demo      *demoOps
	assets    *assets.Module
	equipment equipmentView // other domains' view of the units (IR193)
	blobs     blob.Store
	jobs      maintenance.Jobs
	delivery  maintenance.Delivery
	accounts  identity.Accounts // the daily cap on the accounts a contractor or customer owner adds (IR464)
}

// registerModules registers every module's operations, queries and scheduler jobs. The steps keep their order: the
// scheduler runs each domain's jobs in the order they were added.
func registerModules(
	reg *ops.Registry,
	txm *db.TxManager,
	demo *demoOps,
	accounts identity.Accounts,
) {
	registerDemo(reg, demo)
	identity.Register(reg)
	identity.RegisterMembers(reg)

	assetsModule, equipment := registerEquipment(reg, txm)
	blobs, jobs, delivery := registerMaintenance(reg, equipment)

	wired := &wiring{
		reg:       reg,
		demo:      demo,
		assets:    assetsModule,
		equipment: equipment,
		blobs:     blobs,
		jobs:      jobs,
		delivery:  delivery,
		accounts:  accounts,
	}
	wired.registerControl()
	wired.registerReads()
	wired.registerBilling()
	wired.registerCare()
	wired.registerDevices()
}

// registerEquipment registers the units and their devices and alerts, and returns the assets module with the view
// the other domains have of the units (IR193).
func registerEquipment(reg *ops.Registry, txm *db.TxManager) (*assets.Module, equipmentView) {
	assetsModule := &assets.Module{
		Mon: monitorReads{
			alerts:    monitoring.Alerts{Units: nil, Access: nil, Devices: nil},
			telemetry: monitoring.Telemetry{Sensors: devices.Models{}, Units: nil},
		},
		Models:    devices.Models{},
		Policies:  monitoring.Policies{Units: nil, Recipients: nil, Caps: nil},
		Usage:     []assets.UnitUsage{billing.Usage{}, maintenance.Usage{}, devices.Usage{}},
		Contracts: billing.Usage{},
		Claims:    maintenance.Usage{},
		Import:    devices.Models{},
		Activity: []assets.UnitUsage{
			monitoring.Telemetry{Units: nil, Sensors: nil},
			maintenance.Usage{},
		},
		Store: txm,
		Active: []assets.UnitActivity{
			maintenance.Usage{},
			devices.Usage{},
			restrictions.Busy{},
			control.Busy{},
			billing.Usage{},
		},
		OrgActive: []assets.OrgActivity{billing.Usage{}, maintenance.Usage{}},
		Details:   nil,
	}
	assetsModule.Details = unitDetails{}
	assets.Register(reg, assetsModule)
	// IR340: warranty reminders to HQ (BR-A19)
	reg.AddJob(ops.DomainEquipment, assetsModule.RemindWarranties)
	equipment := equipmentView{
		am:     assetsModule,
		alerts: monitoring.Alerts{Units: nil, Access: nil, Devices: nil},
		tel:    monitoring.Telemetry{Sensors: devices.Models{}, Units: nil},
		models: devices.Models{},
	} // other domains' view (IR193)
	devices.Register(reg, devices.Capabilities{Units: assetsModule})
	devices.RegisterQueries(
		reg,
	) // internal: a device's open connection fault for the IoT gateway (IR532 system query)
	monitoring.RegisterAlerts(
		reg,
		monitoring.Alerts{
			Units:   assetsModule,
			Access:  maintenance.Access{},
			Devices: deviceRecoveries{},
		},
	)

	return assetsModule, equipment
}

// registerMaintenance registers the jobs, their visits, reports, follow-ups, plans, partners, files, delivery and
// proposals, and returns the parts the later steps use.
func registerMaintenance(
	reg *ops.Registry,
	equipment equipmentView,
) (blob.Dir, maintenance.Jobs, maintenance.Delivery) {
	blobs := blob.NewDirFromEnv()
	jobs := maintenance.Jobs{
		Units:    equipment,
		Severity: equipment,
		HQOrg:    func(c *ops.Call) uuid.UUID { return c.Principal.OrgID },
		Sites:    equipment,
		Blobs:    blobs,
	}
	maintenance.RegisterJobs(reg, jobs)
	maintenance.RegisterOnSite(
		reg,
		maintenance.OnSite{Jobs: jobs, Access: maintenance.Access{}},
	)
	reports := maintenance.Reports{Jobs: jobs, Units: equipment, Access: maintenance.Access{}}
	maintenance.RegisterFollowUps(
		reg,
		maintenance.FollowUps{Jobs: jobs, Blobs: blobs, Warranty: equipment},
	)
	maintenance.RegisterReports(reg, reports)
	maintenance.RegisterPlans(reg, maintenance.Plans{Jobs: jobs})
	maintenance.RegisterPartners(
		reg,
		maintenance.Partners{Orgs: identity.Directory{}, Customers: equipment},
	)
	maintenance.RegisterFiles(reg, maintenance.Files{Reports: reports, Blobs: blobs})

	delivery := maintenance.Delivery{
		Jobs:      jobs,
		Directory: identity.Directory{},
		Units:     equipment,
	}
	maintenance.RegisterDelivery(reg, delivery)
	maintenance.RegisterProposals(reg, maintenance.Proposals{Delivery: delivery})

	return blobs, jobs, delivery
}

// registerControl registers the commands, the automations (with the IR54 schedule job) and the diagnostics.
func (w *wiring) registerControl() {
	cmds := control.Commands{
		Units:        controlTargets{w.assets},
		Devices:      devices.Models{},
		Restrictions: restrictions.Busy{},
		Access:       maintenance.Access{},
	}
	control.Register(w.reg, cmds)
	control.RegisterQueries(
		w.reg,
	) // internal: the commands the IoT gateway sends (IR195 system query)

	automations := control.Automations{
		Units:    controlTargets{w.assets},
		Cmd:      cmds,
		Notifier: monitoring.Notifier{Notify: notify.Store{}},
	}
	control.RegisterAutomations(w.reg, automations)
	w.demo.notify = automations.Notify // demo.trigger automation: a gateway's facts against the alert policies (IR529)
	w.reg.AddJob(
		ops.DomainEquipment,
		automations.FireSchedules,
	) // IR54: schedule occurrences fire from the w.equipment scheduler
	control.RegisterDiagnostics(
		w.reg,
		control.Diagnostics{Commands: cmds, Jobs: maintenance.Access{}},
	)
}

// registerReads registers the account settings, writes, QR access, energy, voice, the internal queries of the read
// models (IR190), the summaries and the notifications.
func (w *wiring) registerReads() {
	identity.RegisterPreferences(w.reg)
	identity.RegisterClientUsers(w.reg, w.accounts)
	identity.RegisterPartnerUsers(w.reg, w.accounts)
	identity.RegisterTwoFactor(w.reg)
	writes.Register(w.reg)
	assets.RegisterQr(
		w.reg,
		maintenance.QrAccess{},
	) // an Assets operation; maintenance answers the assignment (IR194)
	energy.Register(w.reg)
	voice.Register(w.reg, voice.Voice{Units: w.assets})
	energy.RegisterMRV(w.reg)
	energy.RegisterOffsets(w.reg)
	sums := summaries.Summaries{Units: w.assets, Registry: w.reg}
	maintenance.RegisterQueries(w.reg, w.jobs) // internal queries of the read models (IR190)
	billing.RegisterQueries(w.reg)
	energy.RegisterQueries(w.reg)
	identity.RegisterQueries(w.reg)
	restrictions.RegisterQueries(w.reg)
	audit.RegisterQueries(w.reg)
	summaries.Register(w.reg, sums)
	summaries.RegisterAdmin(w.reg, sums)
	notify.Register(w.reg)
}

// registerBilling registers the restrictions, billing, payments, inquiries, audit and payouts (with the IR341 job),
// then the cross-domain reads and the workforce.
func (w *wiring) registerBilling() {
	restrictionsModule := restrictions.Restrictions{
		Units:   w.equipment,
		Devices: w.equipment,
		Notify:  notify.Store{},
	}
	restrictions.Register(w.reg, restrictionsModule)
	bill := billing.Billing{
		Customers:    w.equipment,
		Restrictions: restrictions.Busy{},
		Recipients:   identity.Directory{},
		Releases:     restrictionsModule, Notify: notify.Store{},
	}
	billing.Register(w.reg, bill)
	billing.RegisterPayments(w.reg, bill)
	billing.RegisterInquiries(w.reg, bill)
	audit.Register(w.reg)

	maintenanceReads := maintenanceView{src: maintenance.PayoutSource{Jobs: w.jobs}}
	payouts := billing.Payouts{Source: maintenanceReads}
	billing.RegisterPayouts(w.reg, payouts)
	w.reg.AddJob(
		ops.DomainBilling,
		payouts.GenerateMonthly,
	) // IR341: the payout drafts of the month that ended, on the 1st (BR-A23)
	registerCrossDomain(w.reg, w.equipment, maintenanceReads)
	maintenance.RegisterWorkforce(w.reg, maintenance.Workforce{Delivery: w.delivery})
}

// registerCare registers the filter care and certificates with their reminder jobs, and the job that releases a
// technician whose access ends (IR439).
func (w *wiring) registerCare() {
	filters := maintenance.FilterCare{Units: w.equipment, Run: w.equipment}
	maintenance.RegisterFilterCare(w.reg, filters)
	w.reg.AddJob(
		ops.DomainMaintenance,
		filters.Remind,
	) // IR239: filter cleaning reminders from the maintenance scheduler

	certs := maintenance.Certificates{
		Delivery: w.delivery,
		Grants:   identity.Directory{},
		Blobs:    w.blobs,
	}
	maintenance.RegisterCertificates(w.reg, certs)
	w.reg.AddJob(
		ops.DomainMaintenance,
		certs.Remind,
	) // IR336: certificate expiry reminders (BR-P09)
	w.reg.AddJob(
		ops.DomainMaintenance,
		w.jobs.ReleaseEndedAccess,
	) // IR439: a technician whose access ends before the visit is released (OPEN-BE-09)
}

// registerDevices registers the telemetry, alert policies, devices and campaigns, with the device operation and
// campaign jobs after the other equipment jobs.
func (w *wiring) registerDevices() {
	monitoring.RegisterTelemetry(
		w.reg,
		monitoring.Telemetry{Units: w.assets, Sensors: devices.Models{}},
	)
	monitoring.RegisterPolicies(
		w.reg,
		monitoring.Policies{
			Units:      w.assets,
			Recipients: identity.Directory{},
			Caps:       unitCaps{w.assets},
		},
	)
	module := &devices.Module{
		Units: w.assets,
		Exclusion: []devices.Exclusion{
			control.Busy{},
			restrictions.Busy{},
		},
		Access:     nil,
		UnitAccess: nil,
	}
	devices.RegisterDevices(w.reg, module)
	devices.RegisterCampaigns(w.reg, module)
	w.reg.AddJob(
		ops.DomainEquipment,
		devices.Lifecycle{Guard: control.Busy{}}.Advance,
	) // IR67: operations start and time out on the w.equipment scheduler
	w.reg.AddJob(
		ops.DomainEquipment,
		module.AdvanceCampaigns,
	) // IR339: campaign notices, starts, waves and completion (BR-A20), after the timeouts
}

// newAuthenticator verifies the bearer tokens; a service that does not serve identity takes the principals from
// identity-api (IR181 step 1).
func newAuthenticator(
	cfg Config,
	verifier auth.Verifier,
	txm *db.TxManager,
	now func() time.Time,
	servesIdentity bool,
) *auth.Authenticator {
	authn := &auth.Authenticator{Verifier: verifier, DB: txm, Now: now, Source: nil}
	if cfg.IdentityURL == "" || servesIdentity {
		return authn
	}

	ttl := cfg.PrincipalTTL
	if ttl == 0 {
		ttl = defaultPrincipalTTL
	}

	authn.Source = &auth.RemoteSource{
		BaseURL: cfg.IdentityURL,
		Token:   cfg.InternalToken,
		TTL:     ttl, Client: nil, Now: nil,
	}

	return authn
}

// useRemoteQueries points the read models at the owning services when this process serves only some domains (IR190).
func useRemoteQueries(reg *ops.Registry, cfg Config) error {
	if len(cfg.Domains) == 0 || len(cfg.ServiceURLs) == 0 {
		return nil
	}

	httpQueries := &ops.HTTPQueries{
		Targets: map[string]*url.URL{},
		Token:   cfg.InternalToken,
		Client:  nil,
	}
	for domain, raw := range cfg.ServiceURLs {
		u, err := url.Parse(raw)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return fmt.Errorf("server: service URL of %s %w", domain, errRelativeURL)
		}

		httpQueries.Targets[domain] = u
	}

	reg.Remote = httpQueries

	return nil
}

// defaultPrincipalTTL is how long a principal from identity-api is kept when the configuration sets no PrincipalTTL.
const defaultPrincipalTTL = 30 * time.Second

// newRegistry is the registry of the served domains with the configured recorder, idempotency store, limits and
// clock.
func newRegistry(cfg Config, txm *db.TxManager) *ops.Registry {
	reg := ops.NewRegistry()
	reg.ServeDomains(cfg.Domains...)
	reg.DB, reg.Rec, reg.Idem = txm, notifyingRecorder{
		inner: db.Recorder{},
		store: notify.Store{},
	}, db.Idempotency{
		M: txm,
	}
	reg.Limit = cfg.WriteLimit

	reg.DenyLimit = cfg.DenyLimit
	if reg.DenyLimit == nil {
		// without the shared store each process keeps its own minute counts (tests, a single instance)
		reg.DenyLimit = &ratelimit.Window{
			Store:  &ratelimit.Memory{},
			Limit:  1,
			Window: time.Minute,
			Prefix: "ac:rl:denied:", Now: nil,
		}
	}

	if cfg.Clock != nil {
		reg.Clock = cfg.Clock
	}

	return reg
}

// DrainEvents applies every pending event of this server's consumers (inline mode and tests).
func (s *Server) DrainEvents(ctx context.Context) error {
	for range 10 { // events can trigger further events: repeat until nothing is pending
		applied := 0
		errs := make([]error, 0, len(s.Consumers))

		for _, c := range s.Consumers {
			// each on its own, as each runs its own loop: one failing does not hold the others (IR511)
			n, err := c.Drain(ctx)
			errs = append(errs, err)
			applied += n
		}

		err := errors.Join(errs...)
		if err != nil || applied == 0 {
			return err
		}
	}

	return nil
}

// inlineEvents applies the pending events after a write when one process serves every domain; a failure is logged.
func (s *Server) inlineEvents(logger *slog.Logger) func(context.Context) {
	return func(ctx context.Context) {
		err := s.DrainEvents(context.WithoutCancel(ctx))
		if err != nil && logger != nil {
			logger.Error("inline events", "error", err)
		}
	}
}

// monitorReads combines the Monitoring read interfaces Assets uses.
// deviceRecoveries hands the devices' recovery events of an alert to the alert resolution (IR327).
type deviceRecoveries struct{}

func (deviceRecoveries) AlertRecoveries(
	ctx context.Context,
	c *ops.Call,
	alert uuid.UUID,
	since time.Time,
) ([]monitoring.DeviceRecovery, error) {
	rs, err := devices.Models{}.AlertRecoveries(ctx, c, alert, since)

	out := make([]monitoring.DeviceRecovery, len(rs))
	for i, r := range rs {
		out[i] = monitoring.DeviceRecovery{
			ID:         r.ID,
			EventType:  r.EventType,
			OccurredAt: r.OccurredAt,
		}
	}

	return out, apperr.Wrap(err, "devices.Models.AlertRecoveries")
}

type monitorReads struct {
	alerts    monitoring.Alerts
	telemetry monitoring.Telemetry
}

func (m monitorReads) PowerMeasured(
	ctx context.Context,
	c *ops.Call,
	units []uuid.UUID,
) (map[uuid.UUID]bool, error) {
	out, err := m.telemetry.PowerMeasured(ctx, c, units)

	return out, apperr.Wrap(err, "monitoring.Telemetry.PowerMeasured")
}

func (m monitorReads) ActiveAlertCounts(
	ctx context.Context,
	c *ops.Call,
	units []uuid.UUID,
) (map[uuid.UUID]int, error) {
	out, err := m.alerts.ActiveAlertCounts(ctx, c, units)

	return out, apperr.Wrap(err, "monitoring.Alerts.ActiveAlertCounts")
}

func (m monitorReads) LatestMeasurements(
	ctx context.Context,
	c *ops.Call,
	units []uuid.UUID,
) (map[uuid.UUID][]any, error) {
	latest, err := m.telemetry.LatestMeasurements(ctx, c, units)

	out := make(map[uuid.UUID][]any, len(latest))
	for u, ms := range latest {
		for _, x := range ms {
			out[u] = append(out[u], x)
		}
	}

	return out, apperr.Wrap(err, "monitoring.Telemetry.LatestMeasurements")
}

// unitCaps joins the unit's model version (Assets) with its Capability (Devices) for UnitAction checks.
type unitCaps struct{ am *assets.Module }

func (u unitCaps) UnitCaps(
	ctx context.Context,
	call *ops.Call,
	unit uuid.UUID,
) (control.Caps, bool, error) {
	info, ok, err := u.am.UnitInfo(ctx, call, unit)
	if err != nil || !ok {
		return control.Caps{}, false, apperr.Wrap(err, "assets.Module.UnitInfo")
	}

	out, out2, err := devices.Models{}.Caps(ctx, call, info.ModelID, info.CapabilityVersion)

	return out, out2, apperr.Wrap(err, "devices.Models.Caps")
}

// restrictionTargets joins Assets (organization, archived) with the unit's capability for Restrictions.
type restrictionTargets struct{ am *assets.Module }

func (t restrictionTargets) RestrictionTarget(
	ctx context.Context,
	call *ops.Call,
	unit uuid.UUID,
) (restrictions.UnitCaps, bool, error) {
	target, ok, err := controlTargets(t).Target(ctx, call, unit)
	if err != nil || !ok {
		return restrictions.UnitCaps{}, false, err
	}

	org, archived, _, err := t.am.UnitState(ctx, call, unit)
	if err != nil {
		return restrictions.UnitCaps{}, false, apperr.Wrap(err, "assets.Module.UnitState")
	}

	return restrictions.UnitCaps{
		OrgID:          org,
		Archived:       archived,
		Control:        target.Caps.Control,
		HasTemperature: target.Caps.HasTemperature,
		TempMin:        target.Caps.TempMin,
		TempMax:        target.Caps.TempMax,
		Step:           target.Caps.TempStep,
	}, true, nil
}

// controlTargets joins Assets (organization, property, version) with the unit's capability for Control.
type controlTargets struct{ am *assets.Module }

func (t controlTargets) Target(
	ctx context.Context,
	call *ops.Call,
	unit uuid.UUID,
) (control.Target, bool, error) {
	info, ok, err := t.am.UnitInfo(ctx, call, unit)
	if err != nil || !ok {
		return control.Target{}, false, apperr.Wrap(err, "assets.Module.UnitInfo")
	}

	version, err := t.am.UnitVersion(ctx, call, unit)
	if err != nil {
		return control.Target{}, false, apperr.Wrap(err, "assets.Module.UnitVersion")
	}

	caps, _, err := devices.Models{}.Caps(ctx, call, info.ModelID, info.CapabilityVersion)

	return control.Target{
		OrgID:      info.OrgID,
		PropertyID: info.PropertyID,
		Version:    version,
		Caps:       caps,
	}, true, apperr.Wrap(err, "devices.Models.Caps")
}
