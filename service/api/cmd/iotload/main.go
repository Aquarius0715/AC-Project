// Command iotload is the IoT load generator of the load test (docs/02-design/load-test.md, phase 3). Against the
// broker of the throwaway stack ac-load it plays the modules of the devices loadseed registered: every device sends a
// telemetry batch each interval and a heartbeat each heartbeat interval, spread evenly (so 3,000 devices a minute are
// 50 batches a second, and every 9 seconds are about 333), and answers a command on its command topic at once, as
// cmd/devicesim does. Beside this, a few probe devices send one batch every probe interval and the generator polls the
// database for each until it is stored: the time from publishing to storing is the delay of the whole path (broker,
// bridge, Core API). When the run ends it waits for the backlog to drain, counts the readings stored against those
// expected, and writes a report. It writes to no database but ac_load.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"hash/fnv"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pradita/ac-project/service/api/internal/devicesim"
	"github.com/pradita/ac-project/service/api/internal/iotload"
	"github.com/pradita/ac-project/service/api/internal/loadseed"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

const (
	publishWorkers   = 32
	publishQueue     = 8192
	publishWait      = 10 * time.Second
	probeWait        = 30 * time.Second
	probePoll        = 25 * time.Millisecond
	keepAlive        = 60 * time.Second
	disconnectWait   = 500
	statusEvery      = 10 * time.Second
	drainPoll        = time.Second
	startMargin      = 5 * time.Second
	dbConnections    = 4
	readingsPerBatch = 2
	topicParts       = 5     // t/<tenant>/d/<device>/cmd
	reportMode       = 0o600 // the report is for the user only
	loopCount        = 3     // telemetry, heartbeats and probes

	// the probe's reading by its sensor and sequence number, and the readings stored since a time.
	probeQuery  = `SELECT 1 FROM monitoring.measurements WHERE sensor_id = $1 AND sequence = $2`
	storedQuery = `SELECT count(*) FROM monitoring.measurements WHERE received_at >= $1`
)

var (
	errNoDevices = errors.New("the links file names no device to play")
	errNoProbes  = errors.New("the links file names no probe device")
)

type config struct {
	broker, tenant, links, db, out                      string
	interval, heartbeat, probePeriod, duration, drainBy time.Duration
}

// linkRecord is an entry of the links file loadseed writes.
type linkRecord struct {
	Device  string            `json:"device"`
	Unit    string            `json:"unit"`
	Sensors map[string]string `json:"sensors"`
	Probe   bool              `json:"probe"`
}

// sim is one module and the lock that keeps its telemetry loop and its command answers apart.
type sim struct {
	mu         sync.Mutex
	module     *devicesim.Module
	lastMinute time.Time // the minute of the last batch, to count a minute's readings once
	sensors    int       // how many readings a batch has
}

// probe is a probe module and the ID of its temperature sensor, which is looked up in the database.
type probe struct {
	sim

	sensor string
}

// job is one message waiting for a publisher.
type job struct {
	topic   string
	payload []byte
	batch   bool // a telemetry batch, whose readings are counted against the database
}

func main() {
	var cfg config

	flag.StringVar(
		&cfg.broker,
		"broker",
		envOr("MQTT_URL", "tcp://localhost:1883"),
		"MQTT broker URL",
	)
	flag.StringVar(
		&cfg.tenant,
		"tenant",
		seed.ID("tenant-a").String(),
		"tenant the devices belong to",
	)
	flag.StringVar(
		&cfg.links,
		"links",
		os.Getenv("IOT_LINKS_FILE"),
		"the links file loadseed wrote",
	)
	flag.StringVar(
		&cfg.db,
		"db",
		os.Getenv("OWNER_DATABASE_URL"),
		"owner database URL; the database must be "+loadseed.Database,
	)
	flag.StringVar(&cfg.out, "out", "iotload.json", "where the report is written")
	flag.DurationVar(
		&cfg.interval,
		"interval",
		time.Minute,
		"time between the batches of one device (60s: 50 a second for 3,000)",
	)
	flag.DurationVar(
		&cfg.heartbeat,
		"heartbeat",
		time.Minute,
		"time between the heartbeats of one device",
	)
	flag.DurationVar(
		&cfg.probePeriod,
		"probe-period",
		time.Minute,
		"time between the batches of one probe (a sensor keeps one reading a minute, so not less)",
	)
	flag.DurationVar(&cfg.duration, "duration", 0, "how long to generate (0: until a signal)")
	flag.DurationVar(
		&cfg.drainBy,
		"drain",
		time.Minute,
		"how long to wait for the backlog to be stored once generation has ended",
	)
	flag.Parse()

	err := run(cfg)
	if err != nil {
		log.Fatal(err)
	}
}

func run(cfg config) error {
	err := loadseed.Guard(cfg.db)
	if err != nil {
		return apperr.Wrap(err, "loadseed.Guard")
	}

	devices, probes, err := readLinks(cfg)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := openPool(ctx, cfg.db)
	if err != nil {
		return err
	}
	defer pool.Close()

	gen := &generator{
		cfg:        cfg,
		devices:    devices,
		probes:     probes,
		pool:       pool,
		client:     nil, // set once connected: the connect handler uses the generator
		jobs:       iotload.NewQueue[job](publishQueue),
		published:  atomic.Int64{},
		batches:    atomic.Int64{},
		failed:     atomic.Int64{},
		dropped:    atomic.Int64{},
		beats:      atomic.Int64{},
		answered:   atomic.Int64{},
		probesSent: atomic.Int64{},
		slots:      atomic.Int64{},
		mu:         sync.Mutex{},
		arrivals:   nil,
	}

	client := mqtt.NewClient(gen.options())

	tok := client.Connect()
	if tok.Wait() && tok.Error() != nil {
		return fmt.Errorf("connect %s: %w", cfg.broker, tok.Error())
	}
	defer client.Disconnect(disconnectWait)

	gen.client = client

	return gen.play(ctx)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return def
}

func seedOf(id string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(id))

	return h.Sum64()
}

// readLinks makes the modules of the devices and the probes the links file names.
func readLinks(cfg config) ([]*sim, []*probe, error) {
	raw, err := os.ReadFile(cfg.links)
	if err != nil {
		return nil, nil, apperr.Wrap(err, "os.ReadFile")
	}

	var file struct {
		Links []linkRecord `json:"links"`
	}

	err = json.Unmarshal(raw, &file)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", cfg.links, err)
	}

	var (
		devices []*sim
		probes  []*probe
	)

	for _, link := range file.Links {
		id := seed.ID(link.Device).String()
		names := make([]string, 0, len(link.Sensors))

		for name := range link.Sensors {
			names = append(names, name)
		}

		if link.Probe {
			// a probe reports its temperature only, which is what it looks up
			module := devicesim.NewModule(cfg.tenant, id, "v1", seedOf(id)).Only("temperature")
			probes = append(
				probes,
				&probe{
					sim: sim{
						mu:         sync.Mutex{},
						module:     module,
						lastMinute: time.Time{},
						sensors:    0,
					},
					sensor: seed.ID(link.Sensors["temperature"]).String(),
				},
			)

			continue
		}

		module := devicesim.NewModule(cfg.tenant, id, "v1", seedOf(id)).Only(names...)
		devices = append(devices, &sim{
			mu:         sync.Mutex{},
			module:     module,
			lastMinute: time.Time{},
			sensors:    len(names),
		})
	}

	if len(devices) == 0 {
		return nil, nil, errNoDevices
	}

	if len(probes) == 0 {
		return nil, nil, errNoProbes
	}

	return devices, probes, nil
}

func openPool(ctx context.Context, url string) (*pgxpool.Pool, error) {
	conf, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, apperr.Wrap(err, "pgxpool.ParseConfig")
	}

	conf.MaxConns = dbConnections

	pool, err := pgxpool.NewWithConfig(ctx, conf)
	if err != nil {
		return nil, apperr.Wrap(err, "pgxpool.NewWithConfig")
	}

	var current string

	err = pool.QueryRow(ctx, `SELECT current_database()`).Scan(&current)
	if err != nil {
		pool.Close()

		return nil, apperr.Wrap(err, "current_database")
	}

	if current != loadseed.Database { // the URL can name one database and the server answer another
		pool.Close()

		return nil, fmt.Errorf("connected to %q: %w", current, loadseed.ErrWrongDatabase)
	}

	return pool, nil
}

// generator is one run.
type generator struct {
	cfg     config
	devices []*sim
	probes  []*probe
	pool    *pgxpool.Pool
	client  mqtt.Client
	jobs    *iotload.Queue[job]

	published, batches, failed, dropped, beats, answered, probesSent, slots atomic.Int64

	mu       sync.Mutex
	arrivals []time.Duration
}

func (g *generator) options() *mqtt.ClientOptions {
	byDevice := map[string]*sim{}
	for _, dev := range g.devices {
		byDevice[dev.module.DeviceID] = dev
	}

	// t/<tenant>/d/+/cmd: every device's commands
	topic := devicesim.Topic(g.cfg.tenant, "+", "cmd")
	answer := func(_ mqtt.Client, msg mqtt.Message) { g.answer(byDevice, msg) }

	return mqtt.NewClientOptions().AddBroker(g.cfg.broker).SetClientID("iotload").
		SetUsername(os.Getenv("MQTT_USER")).SetPassword(os.Getenv("MQTT_PASSWORD")).
		SetAutoReconnect(true).SetConnectRetry(true).SetKeepAlive(keepAlive).
		SetOnConnectHandler(func(conn mqtt.Client) {
			tok := conn.Subscribe(topic, 1, answer)
			if tok.Wait() && tok.Error() != nil {
				log.Printf("subscribe %s: %v", topic, tok.Error())
			}
		}).
		SetConnectionLostHandler(func(_ mqtt.Client, err error) { log.Printf("broker: connection lost: %v", err) })
}

// answer acknowledges one command at once, as a healthy module does.
func (g *generator) answer(byDevice map[string]*sim, msg mqtt.Message) {
	parts := strings.Split(msg.Topic(), "/") // t/<tenant>/d/<device>/cmd
	if len(parts) != topicParts {
		return
	}

	dev := byDevice[parts[3]]
	if dev == nil {
		return
	}

	dev.mu.Lock()
	raw, _, err := dev.module.Acknowledge(msg.Payload(), time.Now())
	dev.mu.Unlock()

	if err != nil || raw == nil {
		return
	}

	g.enqueue(dev.module.AckTopic(), raw, false)
	g.answered.Add(1)
}

// enqueue hands a message to the publishers; a full or closed queue drops it and counts it, so a slow broker shows as
// a shortfall and not as a generator that falls behind its own schedule.
func (g *generator) enqueue(topic string, payload []byte, batch bool) {
	if !g.jobs.Offer(job{topic: topic, payload: payload, batch: batch}) {
		g.dropped.Add(1)
	}
}

// publisher sends the queued messages at QoS 1 and counts the outcome.
func (g *generator) publisher(wg *sync.WaitGroup) {
	defer wg.Done()

	for j := range g.jobs.Values() {
		tok := g.client.Publish(j.topic, 1, false, j.payload)
		if tok.WaitTimeout(publishWait) && tok.Error() == nil {
			g.published.Add(1)

			if j.batch {
				g.batches.Add(1)
			}
		} else {
			g.failed.Add(1)
		}
	}
}

// play runs the generator until the duration ends or a signal comes, then drains and reports.
func (g *generator) play(ctx context.Context) error {
	started := time.Now()

	runCtx := ctx

	if g.cfg.duration > 0 {
		var cancel context.CancelFunc

		runCtx, cancel = context.WithTimeout(ctx, g.cfg.duration)
		defer cancel()
	}

	var publishers, loops sync.WaitGroup

	for range publishWorkers {
		publishers.Add(1)

		go g.publisher(&publishers)
	}

	loops.Add(loopCount)

	go func() { defer loops.Done(); g.telemetry(runCtx) }()
	go func() { defer loops.Done(); g.heartbeats(runCtx) }()
	go func() { defer loops.Done(); g.probing(runCtx) }()

	log.Printf(
		"iotload: %d devices every %s (%.0f a second), %d probes",
		len(g.devices),
		g.cfg.interval,
		iotload.RatePerSecond(g.cfg.interval, len(g.devices)),
		len(g.probes),
	)

	g.status(runCtx)

	// the time the generator ran, not the time the last probes were waited for
	generated := time.Since(started)

	loops.Wait()
	g.jobs.Close()
	publishers.Wait()

	log.Printf(
		"iotload: generation ended after %s; waiting for the backlog to be stored",
		generated.Round(time.Second),
	)

	// the report is made after a signal as well, so the queries do not follow the cancelled context
	report := g.finish(context.WithoutCancel(ctx), started, generated)

	return writeReport(g.cfg.out, report)
}

// status logs the counters now and then, until the run's context ends.
func (g *generator) status(ctx context.Context) {
	ticker := time.NewTicker(statusEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			g.mu.Lock()
			arrived := len(g.arrivals)
			g.mu.Unlock()

			log.Printf(
				"iotload: published %d, failed %d, dropped %d, probes %d sent / %d arrived",
				g.published.Load(),
				g.failed.Load(),
				g.dropped.Load(),
				g.probesSent.Load(),
				arrived,
			)
		}
	}
}

// telemetry sends one batch from the next device on every tick, round robin.
func (g *generator) telemetry(ctx context.Context) {
	spacing := iotload.Spacing(g.cfg.interval, len(g.devices))
	if spacing <= 0 {
		return
	}

	ticker := time.NewTicker(spacing)
	defer ticker.Stop()

	next := 0

	for {
		select {
		case <-ctx.Done():
			return
		case tick := <-ticker.C:
			dev := g.devices[next%len(g.devices)]
			next++

			dev.mu.Lock()
			raw, err := dev.module.Telemetry(tick)
			minute := tick.UTC().Truncate(time.Minute)
			fresh := !minute.Equal(dev.lastMinute) // the first batch of the device in this minute
			dev.lastMinute = minute
			dev.mu.Unlock()

			if err == nil && raw != nil {
				g.enqueue(dev.module.TelemetryTopic(), raw, true)

				// the platform keeps one reading of a sensor for a minute, so later batches of the minute add none
				if fresh {
					g.slots.Add(int64(dev.sensors))
				}
			}
		}
	}
}

// heartbeats sends one heartbeat from the next device on every tick, round robin.
func (g *generator) heartbeats(ctx context.Context) {
	spacing := iotload.Spacing(g.cfg.heartbeat, len(g.devices))
	if spacing <= 0 {
		return
	}

	ticker := time.NewTicker(spacing)
	defer ticker.Stop()

	next := 0

	for {
		select {
		case <-ctx.Done():
			return
		case tick := <-ticker.C:
			dev := g.devices[next%len(g.devices)]
			next++

			dev.mu.Lock()
			raw, err := dev.module.Beat(tick)
			dev.mu.Unlock()

			if err == nil {
				g.enqueue(dev.module.HeartbeatTopic(), raw, false)
				g.beats.Add(1)
			}
		}
	}
}

// probing sends one probe batch every probe interval and follows each to the database.
func (g *generator) probing(ctx context.Context) {
	// every probe sends once a minute (a sensor keeps one reading for a minute), spread over the minute
	ticker := time.NewTicker(iotload.Spacing(g.cfg.probePeriod, len(g.probes)))
	defer ticker.Stop()

	var watchers sync.WaitGroup
	defer watchers.Wait()

	next := 0

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			target := g.probes[next%len(g.probes)]
			next++

			target.mu.Lock()
			raw, err := target.module.Telemetry(time.Now())
			target.mu.Unlock()

			sequence, ok := firstSequence(raw, err)
			if !ok {
				continue
			}

			sentAt := time.Now()

			tok := g.client.Publish(target.module.TelemetryTopic(), 1, false, raw)
			if !tok.WaitTimeout(publishWait) || tok.Error() != nil {
				g.failed.Add(1)

				continue
			}

			g.probesSent.Add(1)

			watchers.Go(func() {
				// a probe in flight when the run ends is still waited for
				g.await(context.WithoutCancel(ctx), target.sensor, sequence, sentAt)
			})
		}
	}
}

// firstSequence is the sequence number of the first reading of a batch.
func firstSequence(raw []byte, err error) (int64, bool) {
	if err != nil || raw == nil {
		return 0, false
	}

	var batch devicesim.Batch

	if json.Unmarshal(raw, &batch) != nil || len(batch.Measurements) == 0 {
		return 0, false
	}

	return batch.Measurements[0].Sequence, true
}

// await polls for the reading of a probe until it is stored and records the delay; one that is not stored in time is
// lost.
func (g *generator) await(parent context.Context, sensor string, sequence int64, sentAt time.Time) {
	ctx, cancel := context.WithTimeout(parent, probeWait)
	defer cancel()

	ticker := time.NewTicker(probePoll)
	defer ticker.Stop()

	for {
		var one int

		err := g.pool.QueryRow(ctx, probeQuery, sensor, sequence).Scan(&one)
		if err == nil {
			g.mu.Lock()
			g.arrivals = append(g.arrivals, time.Since(sentAt))
			g.mu.Unlock()

			return
		}

		if !errors.Is(err, pgx.ErrNoRows) && ctx.Err() == nil {
			log.Printf("probe %s: %v", sensor, err)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// finish waits for the backlog to be stored (up to the drain time) and builds the report.
func (g *generator) finish(
	ctx context.Context,
	started time.Time,
	elapsed time.Duration,
) iotload.Report {
	expected := g.slots.Load() + g.probesSent.Load() // a probe batch has one reading
	drainStart := time.Now()
	stored := g.stored(ctx, started.Add(-startMargin))

	for stored < expected && time.Since(drainStart) < g.cfg.drainBy {
		time.Sleep(drainPoll)

		stored = g.stored(ctx, started.Add(-startMargin))
	}

	g.mu.Lock()
	arrived := slicesClone(g.arrivals)
	g.mu.Unlock()

	return iotload.Report{
		DurationSeconds: elapsed.Seconds(),
		Devices:         len(g.devices),
		IntervalSeconds: g.cfg.interval.Seconds(),
		TargetPerSecond: iotload.RatePerSecond(
			g.cfg.interval,
			len(g.devices),
		),
		Published:     g.batches.Load(),
		PublishFailed: g.failed.Load(),
		Dropped:       g.dropped.Load(),
		Heartbeats:    g.beats.Load(),
		Commands:      g.answered.Load(),
		Expected:      expected,
		Stored:        stored,
		DrainSeconds:  time.Since(drainStart).Seconds(),
		Probes:        iotload.Summarize(arrived, int(g.probesSent.Load())),
	}
}

func slicesClone(in []time.Duration) []time.Duration { return append([]time.Duration(nil), in...) }

// stored counts the readings the database holds since the run began.
func (g *generator) stored(ctx context.Context, since time.Time) int64 {
	var n int64

	err := g.pool.QueryRow(ctx, storedQuery, since).Scan(&n)
	if err != nil {
		log.Printf("count readings: %v", err)
	}

	return n
}

func writeReport(path string, report iotload.Report) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return apperr.Wrap(err, "json.MarshalIndent")
	}

	log.Printf("iotload: %s", data)

	return apperr.Wrap(os.WriteFile(path, data, reportMode), "os.WriteFile")
}
