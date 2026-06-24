/*
 */
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/miekg/dns"
	"github.com/modelos/modelos/node/chaincfg"
	"github.com/modelos/modelos/node/wire"
)

// NodeCounts holds various statistics about the running system for use in html templates
type NodeCounts struct {
	NdStatus  []uint32     // number of nodes at each of the 4 statuses - RG, CG, WG, NG
	NdStarts  []uint32     // number of crawles started last startcrawlers run
	DNSCounts []uint32     // number of dns requests for each dns type - dnsV4Std, dnsV4Non, dnsV6Std, dnsV6Non
	mtx       sync.RWMutex // protect the structures
}

// configData holds information on the application
type configData struct {
	dnsUnknown uint64                // the number of dns requests for we are not configured to handle
	uptime     time.Time             // application start time
	port       string                // port for the dns server to listen on
	http       string                // port for the web server to listen on
	version    string                // application version
	seeders    map[string]*dnsseeder // holds a pointer to all the current seeders
	smtx       sync.RWMutex          // protect the seeders map
	order      []string              // the order of loading the netfiles so we can display in this order
	dns        map[string][]dns.RR   // holds details of all the currently served dns records
	dnsmtx     sync.RWMutex          // protect the dns map
	verbose    bool                  // verbose output cmdline option
	debug      bool                  // debug cmdline option
	stats      bool                  // stats cmdline option
}

var config configData
var netfile string
var testnet bool
var testnet2 bool
var dnsHost string
var initialIP string

func main() {

	var j bool

	config.version = "0.9.1"
	config.uptime = time.Now()

	flag.StringVar(&netfile, "netfile", "", "List of json config files to load")
	flag.StringVar(&config.port, "p", "8053", "DNS Port to listen on")
	flag.StringVar(&config.http, "w", "", "Web Port to listen on. No port specified & no web server running")
	flag.BoolVar(&j, "j", false, "Write network template file (dnsseeder.json) and exit")
	flag.BoolVar(&config.verbose, "v", false, "Display verbose output")
	flag.BoolVar(&config.debug, "d", false, "Display debug output")
	flag.BoolVar(&config.stats, "s", false, "Display stats output")
	flag.BoolVar(&testnet, "testnet", false, "Use TestNet parameters")
	flag.BoolVar(&testnet2, "testnet2", false, "Use TestNet2 parameters")
	flag.StringVar(&dnsHost, "host", "", "DNS hostname to serve (e.g. seed.pearl.org)")
	flag.StringVar(&initialIP, "i", "", "Comma separated list of initial IPs to crawl")
	flag.Parse()

	if j == true {
		createNetFile()
		fmt.Printf("Template file has been created\n")
		os.Exit(0)
	}

	config.seeders = make(map[string]*dnsseeder)
	config.dns = make(map[string][]dns.RR)
	config.order = []string{}

	// Load from netfile if specified
	if netfile != "" {
		netwFiles := strings.Split(netfile, ",")
		for _, nwFile := range netwFiles {
			nnw, err := loadNetwork(nwFile)
			if err != nil {
				fmt.Printf("Error loading data from netfile %s - %v\n", nwFile, err)
				os.Exit(1)
			}
			if nnw != nil {
				config.seeders[nnw.name] = nnw
				config.order = append(config.order, nnw.name)
			}
		}
	} else {
		// Load from params
		if dnsHost == "" {
			fmt.Printf("Error - No netfile and no host specified.\n")
			flag.Usage()
			os.Exit(1)
		}

		var params *chaincfg.Params
		if testnet2 {
			params = &chaincfg.TestNet2Params
		} else if testnet {
			params = &chaincfg.TestNetParams
		} else {
			params = &chaincfg.MainNetParams
		}

		s := initSeederFromParams(params, dnsHost, initialIP)
		config.seeders[s.name] = s
		config.order = append(config.order, s.name)
	}

	if config.debug == true {
		config.verbose = true
		config.stats = true
	}
	if config.verbose == true {
		config.stats = true
	}

	for _, v := range config.seeders {
		log.Printf("status - system is configured for network: %s\n", v.name)
	}

	if config.verbose == false {
		log.Printf("status - Running in quiet mode with limited output produced\n")
	}

	// start the web interface if we want it running
	if config.http != "" {
		log.Printf("status - Starting Web server on port: %s\n", config.http)
		go startHTTP(config.http)
	}

	// start dns server
	log.Printf("status - Starting DNS server on port: %s (UDP/TCP)\n", config.port)
	dns.HandleFunc(".", handleDNS)
	go serve("udp", config.port)
	// RFC 7766 Sec. 5: "Authoritative server implementations MUST support TCP"
	go serve("tcp", config.port)

	var wg sync.WaitGroup

	done := make(chan struct{})
	// start a goroutine for each seeder
	for _, s := range config.seeders {
		wg.Add(1)
		go s.runSeeder(done, &wg)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	// block until a signal is received
	fmt.Println("\nShutting down on signal:", <-sig)

	// FIXME - call dns server.Shutdown()

	// close the done channel to signal to all seeders to shutdown
	// and wait for them to exit
	close(done)
	wg.Wait()
	fmt.Printf("\nProgram exiting. Bye\n")
}

func initSeederFromParams(params *chaincfg.Params, dnsHost string, initialIP string) *dnsseeder {
	s := &dnsseeder{
		id:          params.Net,
		chainParams: params,
		theList:     make(map[string]*node),
		dnsHost:     dnsHost,
		name:        params.Name,
		desc:        params.Name,
		pver:        wire.ProtocolVersion,
		ttl:         600,
		maxSize:     1250,
		maxStart:    []uint32{20, 20, 20, 30},
		delay:       []int64{210, 789, 234, 1876},
	}

	// Port conversion
	port, _ := strconv.Atoi(params.DefaultPort)
	s.port = uint16(port)

	// Initial IPs from params.DNSSeeds
	for _, seed := range params.DNSSeeds {
		if seed.Host != "" {
			s.seeders = append(s.seeders, seed.Host)
		}
	}

	// Parse initial IPs from flag
	if initialIP != "" {
		for _, ip := range strings.Split(initialIP, ",") {
			s.initialIPs = append(s.initialIPs, strings.TrimSpace(ip))
		}
	}

	// Init stats
	s.counts.NdStatus = make([]uint32, maxStatusTypes)
	s.counts.NdStarts = make([]uint32, maxStatusTypes)
	s.counts.DNSCounts = make([]uint32, maxDNSTypes)

	return s
}

// updateNodeCounts runs in a goroutine and updates the global stats with the latest
// counts from a startCrawlers run
func updateNodeCounts(s *dnsseeder, tcount uint32, started, totals []uint32) {
	s.counts.mtx.Lock()

	for st := range []int{statusRG, statusCG, statusWG, statusNG} {
		if config.stats {
			log.Printf("%s: started crawler: %s total: %v started: %v\n", s.name, status2str(uint32(st)), totals[st], started[st])
		}

		// update the stats counters
		s.counts.NdStatus[st] = totals[st]
		s.counts.NdStarts[st] = started[st]
	}

	if config.stats {
		log.Printf("%s: crawlers started. total nodes: %d\n", s.name, tcount)
	}
	s.counts.mtx.Unlock()
}

// status2str will return the string description of the status
func status2str(status uint32) string {
	switch status {
	case statusRG:
		return "statusRG"
	case statusCG:
		return "statusCG"
	case statusWG:
		return "statusWG"
	case statusNG:
		return "statusNG"
	default:
		return "Unknown"
	}
}

// updateDNSCounts runs in a goroutine and updates the global stats for the number of DNS requests
func updateDNSCounts(name, qtype string) {
	var ndType uint32
	var counted bool

	nonstd := strings.HasPrefix(name, "nonstd.")

	switch qtype {
	case "A":
		if nonstd {
			ndType = dnsV4Non
		} else {
			ndType = dnsV4Std
		}
	case "AAAA":
		if nonstd {
			ndType = dnsV6Non
		} else {
			ndType = dnsV6Std
		}
	default:
		ndType = dnsInvalid
	}

	// for DNS requests we do not have a reference to a seeder so we have to find it
	for _, s := range config.seeders {
		s.counts.mtx.Lock()

		if name == s.dnsHost+"." || name == "nonstd."+s.dnsHost+"." {
			s.counts.DNSCounts[ndType]++
			counted = true
		}
		s.counts.mtx.Unlock()
	}
	if counted != true {
		atomic.AddUint64(&config.dnsUnknown, 1)
	}
}

/*

 */
