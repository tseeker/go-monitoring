package main

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"nocternity.net/go/monitoring/perfdata"
	"nocternity.net/go/monitoring/plugin"

	"github.com/karrick/golf"
	"github.com/miekg/dns"
)

//-------------------------------------------------------------------------------------------------------

type (
	// A response to a DNS query. Includes the actual response, the RTT and the error, if any.
	queryResponse struct {
		data *dns.Msg
		rtt  time.Duration
		err  error
	}

	// A channel that can be used to send DNS query responses back to the caller.
	responseChannel chan<- queryResponse
)

// Query a zone's SOA record through a given DNS and return the response using the channel.
func queryZoneSOA(dnsq *dns.Msg, hostname string, port int, output responseChannel) {
	dnsc := new(dns.Client)
	in, rtt, err := dnsc.Exchange(dnsq, fmt.Sprintf("%s:%d", hostname, port))
	output <- queryResponse{
		data: in,
		rtt:  rtt,
		err:  err,
	}
}

//-------------------------------------------------------------------------------------------------------

// Command line flags that have been parsed.
type programFlags struct {
	hostname   string // DNS to check - hostname
	port       int    // DNS to check - port
	zone       string // Zone name
	rsHostname string // Reference DNS - hostname
	rsPort     int    // Reference DNS - port
}

// Program data including configuration and runtime data.
type checkProgram struct {
	programFlags                // Flags from the command line
	plugin       *plugin.Plugin // Plugin output state
}

// Parse command line arguments and store their values. If the -h flag is present,
// help will be displayed and the program will exit.
func (flags *programFlags) parseArguments() {
	var help bool
	golf.BoolVarP(&help, 'h', "help", false, "Display usage information")
	golf.StringVarP(&flags.hostname, 'H', "hostname", "", "Hostname of the DNS to check.")
	golf.IntVarP(&flags.port, 'P', "port", 53, "Port number of the DNS to check.")
	golf.StringVarP(&flags.zone, 'z', "zone", "", "Zone name.")
	golf.StringVarP(&flags.rsHostname, 'r', "rs-hostname", "", "Hostname of the reference DNS.")
	golf.IntVarP(&flags.rsPort, 'p', "rs-port", 53, "Port number of the reference DNS.")
	golf.Parse()
	if help {
		golf.Usage()
		os.Exit(0)
	}
}

// Initialise the monitoring check program.
func newProgram() *checkProgram {
	program := &checkProgram{
		plugin: plugin.New("DNS zone serial match check"),
	}
	program.parseArguments()
	return program
}

// Terminate the monitoring check program.
func (program *checkProgram) close() {
	if r := recover(); r != nil {
		program.plugin.SetState(plugin.UNKNOWN, "Internal error")
		program.plugin.AddLine("Error info: %v", r)
	}
	program.plugin.Done()
}

// Check the values that were specified from the command line. Returns true if the arguments made sense.
func (program *checkProgram) checkFlags() bool {
	if program.hostname == "" {
		program.plugin.SetState(plugin.UNKNOWN, "no DNS hostname specified")
		return false
	}
	if program.port < 1 || program.port > 65535 {
		program.plugin.SetState(plugin.UNKNOWN, "invalid DNS port number")
		return false
	}
	if program.zone == "" {
		program.plugin.SetState(plugin.UNKNOWN, "no DNS zone specified")
		return false
	}
	if program.rsHostname == "" {
		program.plugin.SetState(plugin.UNKNOWN, "no reference DNS hostname specified")
		return false
	}
	if program.rsPort < 1 || program.rsPort > 65535 {
		program.plugin.SetState(plugin.UNKNOWN, "invalid reference DNS port number")
		return false
	}
	program.hostname = strings.ToLower(program.hostname)
	program.zone = strings.ToLower(program.zone)
	program.rsHostname = strings.ToLower(program.rsHostname)
	return true
}

// Query both the server to check and the reference server for the zone's SOA record and return both
// responses (checked server response and reference server response, respectively).
func (program *checkProgram) queryServers() (queryResponse, queryResponse) {
	dnsq := new(dns.Msg)
	dnsq.SetQuestion(dns.Fqdn(program.zone), dns.TypeSOA)
	checkOut := make(chan queryResponse)
	refOut := make(chan queryResponse)
	go queryZoneSOA(dnsq, program.hostname, program.port, checkOut)
	go queryZoneSOA(dnsq, program.rsHostname, program.rsPort, refOut)
	var checkResponse, refResponse queryResponse
	for i := 0; i < 2; i++ {
		select {
		case m := <-checkOut:
			checkResponse = m
		case m := <-refOut:
			refResponse = m
		}
	}
	return checkResponse, refResponse
}

// Add a server's RTT to the performance data.
func (program *checkProgram) addRttPerf(name string, value time.Duration) {
	s := fmt.Sprintf("%f", value.Seconds())
	pd := perfdata.New(name, perfdata.UOM_SECONDS, s)
	program.plugin.AddPerfData(pd)
}

func (program *checkProgram) addResponseInfo(server string, response queryResponse) {
}

// Add information about one of the servers' response to the plugin output. This includes
// the error message if the query failed or the RTT performance data if it succeeded. It
// then attempts to extract the serial from a server's response and returns it if
// successful.
func (program *checkProgram) getSerial(server string, response queryResponse) (ok bool, serial uint32) {
	if response.err != nil {
		program.plugin.AddLine("%s server error : %s", server, response.err)
		return false, 0
	}
	program.addRttPerf(fmt.Sprintf("%s_rtt", server), response.rtt)
	if len(response.data.Answer) != 1 {
		program.plugin.AddLine("%s server did not return exactly one record", server)
		return false, 0
	}
	if soa, ok := response.data.Answer[0].(*dns.SOA); ok {
		program.plugin.AddLine("serial on %s server: %d", server, soa.Serial)
		return true, soa.Serial
	}
	t := reflect.TypeOf(response.data.Answer[0])
	program.plugin.AddLine("%s server did not return SOA record; record type: %v", server, t)
	return false, 0
}

// Run the monitoring check. This implies querying both servers, extracting the serial from
// their responses, then comparing the serials.
func (program *checkProgram) runCheck() {
	checkResponse, refResponse := program.queryServers()
	cOk, cSerial := program.getSerial("checked", checkResponse)
	rOk, rSerial := program.getSerial("reference", refResponse)
	if !(cOk && rOk) {
		program.plugin.SetState(plugin.UNKNOWN, "could not read serials")
		return
	}
	if cSerial == rSerial {
		program.plugin.SetState(plugin.OK, "serials match")
	} else {
		program.plugin.SetState(plugin.CRITICAL, "serials mismatch")
	}
}

func main() {
	program := newProgram()
	defer program.close()
	if program.checkFlags() {
		program.runCheck()
	}
}
