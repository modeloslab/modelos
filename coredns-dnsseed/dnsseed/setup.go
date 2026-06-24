package dnsseed

import (
	"context"
	crypto_rand "crypto/rand"
	"math"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"

	"github.com/modelos/modelos/coredns-dnsseed/crawler"
)

const pluginName = "dnsseed"

var (
	log                          = clog.NewWithPlugin(pluginName)
	defaultUpdateInterval        = 15 * time.Minute
	defaultTTL            uint32 = 3600
	defaultMaxAnswers     uint32 = 25
)

func init() { plugin.Register(pluginName, setup) }

type options struct {
	networkName    string
	updateInterval time.Duration
	recordTTL      uint32
	maxAnswers     uint32
	bootstrapPeers []string
}

func setup(c *caddy.Controller) error {
	zone, err := url.Parse(c.Key)
	if err != nil {
		return c.Errf("couldn't parse zone from block identifier: %s", c.Key)
	}

	opts, err := parseConfig(c)
	if err != nil {
		return err
	}

	if len(opts.bootstrapPeers) == 0 {
		return plugin.Error(pluginName, c.Err("config supplied no bootstrap peers"))
	}

	seeder, err := crawler.NewSeeder(opts.networkName)
	if err != nil {
		return plugin.Error(pluginName, err)
	}

	log.Infof("Connecting to bootstrap peers %v", opts.bootstrapPeers)

	connectedToBootstrap := false
	for _, s := range opts.bootstrapPeers {
		address, port, err := net.SplitHostPort(s)
		if err != nil {
			return plugin.Error(pluginName, c.Errf("config error: expected 'host:port', got %s", s))
		}

		addresses, err := net.LookupHost(address)
		if err != nil {
			log.Errorf("error looking up host %s: %v", address, err)
			continue
		}

		for _, addr := range addresses {
			_, err = seeder.Connect(addr, port)
			if err != nil {
				log.Errorf("error connecting to %s:%s: %v", addr, port, err)
			} else {
				log.Infof("connected to bootstrap peer %s:%s", addr, port)
				connectedToBootstrap = true
			}
		}
	}

	if !connectedToBootstrap {
		return plugin.Error(pluginName, c.Err("failed to connect to any bootstrap peers"))
	}

	ctx, cancel := context.WithCancel(context.Background())

	go crawlLoop(ctx, opts.networkName, seeder, opts.updateInterval)

	err = seeder.WaitForAddresses(1, 30*time.Second)
	if err != nil {
		cancel()
		return plugin.Error(pluginName, c.Err("timed out waiting for initial addresses"))
	}

	c.OnShutdown(func() error {
		cancel()
		seeder.DisconnectAllPeers()
		return nil
	})

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		return PearlSeeder{
			Next:   next,
			Zones:  []string{zone.Hostname()},
			seeder: seeder,
			opts:   opts,
		}
	})

	return nil
}

func crawlLoop(ctx context.Context, name string, seeder *crawler.Seeder, interval time.Duration) {
	runCrawl(name, seeder)
	log.Infof("Starting crawl timer on %s, interval %.1fm", name, interval.Minutes())

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	randByte := []byte{0}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runCrawl(name, seeder)
			crypto_rand.Read(randByte[:])
			if randByte[0] >= 192 {
				seeder.RetryBlacklist()
			}
		}
	}
}

func parseConfig(c *caddy.Controller) (*options, error) {
	opts := &options{
		updateInterval: defaultUpdateInterval,
		recordTTL:      defaultTTL,
		maxAnswers:     defaultMaxAnswers,
	}
	c.Next() // skip "dnsseed"

	if !c.NextBlock() {
		return nil, plugin.Error(pluginName, c.SyntaxErr("expected config block"))
	}

	for loaded := true; loaded; loaded = c.NextBlock() {
		switch c.Val() {
		case "network":
			if !c.NextArg() {
				return nil, plugin.Error(pluginName, c.SyntaxErr("no network specified"))
			}
			opts.networkName = c.Val()
			switch opts.networkName {
			case "mainnet", "testnet", "testnet2", "regtest", "signet", "simnet":
			default:
				return nil, plugin.Error(pluginName, c.SyntaxErr(
					"networks are {mainnet, testnet, testnet2, regtest, signet, simnet}"))
			}

		case "crawl_interval":
			if !c.NextArg() {
				return nil, plugin.Error(pluginName, c.SyntaxErr("no crawl interval specified"))
			}
			interval, err := time.ParseDuration(c.Val())
			if err != nil || interval == 0 {
				return nil, plugin.Error(pluginName, c.SyntaxErr("bad crawl_interval duration"))
			}
			opts.updateInterval = interval

		case "bootstrap_peers":
			bootstrap := c.RemainingArgs()
			if len(bootstrap) == 0 {
				return nil, plugin.Error(pluginName, c.SyntaxErr("no bootstrap peers specified"))
			}
			opts.bootstrapPeers = bootstrap

		case "record_ttl":
			if !c.NextArg() {
				return nil, plugin.Error(pluginName, c.SyntaxErr("no ttl specified"))
			}
			ttl, err := strconv.Atoi(c.Val())
			if err != nil || ttl <= 0 || ttl > math.MaxUint32 {
				return nil, plugin.Error(pluginName, c.SyntaxErr("bad ttl"))
			}
			opts.recordTTL = uint32(ttl)

		case "max_answers":
			if !c.NextArg() {
				return nil, plugin.Error(pluginName, c.SyntaxErr("no max_answers specified"))
			}
			n, err := strconv.Atoi(c.Val())
			if err != nil || n <= 0 || n > 100 {
				return nil, plugin.Error(pluginName, c.SyntaxErr("bad max_answers (1-100)"))
			}
			opts.maxAnswers = uint32(n)

		default:
			return nil, plugin.Error(pluginName, c.SyntaxErr("unsupported option"))
		}
	}

	return opts, nil
}

func runCrawl(name string, seeder *crawler.Seeder) {
	start := time.Now()
	seeder.RefreshAddresses(false)
	newPeerCount := seeder.RequestAddresses()
	seeder.DisconnectAllPeers()
	elapsed := time.Since(start).Truncate(time.Second).Seconds()
	log.Infof("[%s] crawl complete, met %d new peers of %d total in %.0fs",
		name, newPeerCount, seeder.GetPeerCount(), elapsed)
}
