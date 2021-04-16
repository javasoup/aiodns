package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/AdguardTeam/golibs/log"
	"github.com/AdguardTeam/golibs/utils"
	"github.com/emirpasic/gods/sets/hashset"
	"github.com/urfave/cli"
	"gopkg.in/yaml.v3"
)

var (
	options = Options{
		AllServers:       true,
		EnableEDNSSubnet: true,
		TLSMinVersion:    1.2,
	}

	defaultUpstream = new(cli.StringSlice)
	specUpstream    = new(cli.StringSlice)
	fallUpstream    = new(cli.StringSlice)
	bootUpstream    = new(cli.StringSlice)
)

func init() {
	defaultUpstream.Set("tls://dns.pub")
	defaultUpstream.Set("tls://223.6.6.6")
	defaultUpstream.Set("https://doh.pub/dns-query")
	defaultUpstream.Set("https://dns.alidns.com/dns-query")

	specUpstream.Set("tls://8.8.8.8")
	specUpstream.Set("tls://162.159.36.1")
	// specUpstream.Set("tls://dns.adguard.com")
	// specUpstream.Set("quic://dns.adguard.com")
	specUpstream.Set("https://dns.google/dns-query")
	specUpstream.Set("https://dns11.quad9.net/dns-query")
	specUpstream.Set("https://doh.opendns.com/dns-query")
	specUpstream.Set("https://cloudflare-dns.com/dns-query")

	fallUpstream.Set("tls://d.rubyfish.cn")
	fallUpstream.Set("https://i.233py.com/dns-query")

	bootUpstream.Set("tls://223.5.5.5")
	bootUpstream.Set("tls://1.0.0.1")
	bootUpstream.Set("tls://101.101.101.101")
	bootUpstream.Set("114.114.115.115")
}

func cliErrorExit(c *cli.Context, err error) {
	fmt.Printf("%+v", err)
	cli.ShowAppHelp(c)
	os.Exit(-1)
}

func fetch(uri string, resolvers []string) (dat []byte, err error) {
	// Fetch List (Online or Local)
	if strings.HasPrefix(uri, "http://") || strings.HasPrefix(uri, "https://") {
		log.Printf("Fetching online list: [%s]", uri)
		dat, err = curl(uri, resolvers, 5)
	} else {
		if strings.HasPrefix(uri, "~") {
			homedir, _ := os.UserHomeDir()
			uri = homedir + uri[1:]
		}
		log.Printf("Fetching local list: [%s]", uri)
		dat, err = ioutil.ReadFile(uri)
	}

	// gunzip if needed
	if strings.HasSuffix(uri, ".gz") {
		if zReader, zErr := gzip.NewReader(bytes.NewReader(dat)); zErr == nil {
			dat, _ = ioutil.ReadAll(zReader)
		} else {
			err = zErr
		}
	}
	return
}

func scanDoamins(dat []byte, filter *hashset.Set) (domains *hashset.Set) {
	domains = hashset.New()
	scanner := bufio.NewScanner(bytes.NewReader(dat))
	for scanner.Scan() {
		it := strings.TrimSpace(scanner.Text())
		for strings.HasPrefix(it, "#") {
			continue
		}
		for strings.HasPrefix(it, ".") {
			it = it[1:]
		}
		for strings.HasSuffix(it, ".") && len(it) > 0 {
			it = it[:len(it)-1]
		}
		if match, _ := regexp.MatchString(`^(server|ipset)=/[^\/]*/`, it); match {
			it = it[8:strings.LastIndex(it, `/`)]
		}
		if len(it) <= 0 || (filter != nil && filter.Contains(it)) {
			continue
		}
		if utils.IsValidHostname(it+`.`) == nil {
			fmt.Printf("Domain Skiped: %s\n", it)
			continue
		}
		domains.Add(it)
	}
	return
}

func main() {
	app := cli.NewApp()
	app.Name = "AIO DNS"
	app.Usage = "All In One Clean DNS Solution."
	app.Version = fmt.Sprintf("Git:[%s] (%s)", VersionString, runtime.Version())
	// app.HideVersion = true
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "listen, l",
			Value: ":5300",
			Usage: "Listening address",
		},
		cli.StringSliceFlag{
			Name:  "upstream, u",
			Value: defaultUpstream,
			Usage: "An upstream to be default used (can be specified multiple times)",
		},
		cli.StringSliceFlag{
			Name:  "special-upstream, U",
			Value: specUpstream,
			Usage: "An upstream to be special used (can be specified multiple times)",
		},
		cli.StringSliceFlag{
			Name:  "fallback, f",
			Value: fallUpstream,
			Usage: "Fallback resolvers to use when regular ones are unavailable, can be specified multiple times",
		},
		cli.StringSliceFlag{
			Name:  "bootstrap, b",
			Value: bootUpstream,
			Usage: "Bootstrap DNS for DoH and DoT, can be specified multiple times",
		},
		cli.StringSliceFlag{
			Name:  "special-list, L",
			Usage: "List of domains using special-upstream (can be specified multiple times)",
		},
		cli.StringSliceFlag{
			Name:  "bypass-list, B",
			Usage: "List of domains bypass special-upstream (can be specified multiple times)",
		},
		cli.StringFlag{
			Name:  "edns, e",
			Usage: "Send EDNS Client Address to default upstreams",
		},
		cli.BoolFlag{
			Name:  "cache, C",
			Usage: "If specified, DNS cache is enabled",
		},
		cli.BoolFlag{
			Name:  "insecure, I",
			Usage: "If specified, disable SSL/TLS Certificate check (for some OS without ca-certificates)",
		},
		cli.BoolFlag{
			Name:  "ipv6-disabled, R",
			Usage: "If specified, all AAAA requests will be replied with NoError RCode and empty answer",
		},
		cli.BoolFlag{
			Name:  "refuse-any, A",
			Usage: "If specified, refuse ANY requests",
		},
		cli.BoolFlag{
			Name:  "fastest-addr, F",
			Usage: "If specified, Respond to A or AAAA requests only with the fastest IP address",
		},
		cli.BoolFlag{
			Name:  "verbose, V",
			Usage: "If specified, Verbose output",
		},
		cli.StringFlag{
			Name:  "output, O",
			Usage: "If specified, Verbose output",
		},
	}

	app.Action = func(c *cli.Context) error {
		if !strings.HasPrefix(VersionString, "undefined") {
			fmt.Fprintf(os.Stderr, "%s %s\n", strings.ToUpper(c.App.Name), c.App.Version)
		}

		if host, port, err := net.SplitHostPort(c.String("listen")); err != nil {
			cliErrorExit(c, err)
		} else {
			if hostIP := net.ParseIP(host); hostIP != nil {
				options.ListenAddrs = append(options.ListenAddrs, host)
			} else {
				options.ListenAddrs = append(options.ListenAddrs, "0.0.0.0")
			}
			if portInt, err := strconv.Atoi(port); err == nil {
				options.ListenPorts = append(options.ListenPorts, portInt)
			} else {
				cliErrorExit(c, err)
			}
		}

		options.EDNSAddr = c.String("edns")
		options.Cache = c.BoolT("cache")
		options.Verbose = c.BoolT("verbose")
		options.LogOutput = c.String("output")
		options.Insecure = c.BoolT("insecure")
		options.RefuseAny = c.BoolT("refuse-any")
		options.IPv6Disabled = c.BoolT("ipv6-disabled")
		options.FastestAddress = c.BoolT("fastest-addr")
		if options.FastestAddress {
			options.Cache = true
			options.CacheMinTTL = 600
		}
		if options.Cache {
			options.CacheSizeBytes = 4 * 1024 * 1024 // 4M
		}

		options.Upstreams = c.StringSlice("upstream")
		options.Fallbacks = c.StringSlice("fallback")
		options.BootstrapDNS = c.StringSlice("bootstrap")

		specDomains := hashset.New()

		specLists := []string{} // list[domains mulit-lines]
		if len(c.StringSlice("special-list")) > 0 {
			for _, it := range c.StringSlice("special-list") {
				dat, err := fetch(it, options.BootstrapDNS)

				// skip if error
				if err != nil {
					log.Println(err)
					log.Printf("Failed; Skipped! [%s]", it)
					continue
				}

				// append special-list
				specLists = append(specLists, string(dat))
				log.Printf("%d lines special list fetched", len(strings.Split(string(dat), "\n")))
			}
		}

		// FailSafe or Default
		if len(specLists) <= 0 {
			log.Printf("Using build-in special list")
			specLists = append(specLists, specList)
			specLists = append(specLists, tldnList)
		}

		for _, v := range specLists {
			specDomains.Add(scanDoamins([]byte(v), specDomains).Values()...)
		}

		for _, u := range c.StringSlice("special-upstream") {
			for _, it := range specDomains.Values() {
				nUpstream := fmt.Sprintf("[/%s/]%s", it, u)
				options.Upstreams = append(options.Upstreams, nUpstream)
			}
		}

		if len(c.StringSlice("bypass-list")) > 0 {
			bypassDomains := hashset.New()
			for _, it := range c.StringSlice("bypass-list") {
				dat, err := fetch(it, options.BootstrapDNS)

				// skip if error
				if err != nil {
					log.Println(err)
					log.Printf("Failed; Skipped! [%s]", it)
					continue
				}

				// append bypass-list
				bypassDomains.Add(scanDoamins(dat, nil).Values()...)
			}
			cnt := 0
			for _, it := range bypassDomains.Values() {
				needBypass := false
				for _, spec := range specDomains.Values() {
					if strings.HasSuffix(it.(string), `.`+spec.(string)) {
						needBypass = true
						break
					}
				}
				if needBypass {
					nUpstream := fmt.Sprintf("[/%s/]#", it)
					options.Upstreams = append(options.Upstreams, nUpstream)
					cnt++
				}
			}
			log.Printf("%d bypass rules configured, totally", cnt)
		}

		for _, u := range initSpecUpstreams {
			for _, it := range initSpecDomains.Values() {
				options.Upstreams = append(options.Upstreams, fmt.Sprintf("[/%s/]%s", it, u))
			}
		}

		if options.Verbose {
			dump, _ := yaml.Marshal(&options)
			fmt.Println(string(dump))
		} else {
			log.Printf("Speclist Length: %d", specDomains.Size())
			log.Printf("Upstream Rule Count: %d", len(options.Upstreams))
		}

		run(options)
		return nil
	}
	app.Run(os.Args)
}
