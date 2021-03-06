package main

import (
	"bufio"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/textproto"
	"os"
	"strings"
	"time"

	"nocternity.net/go/monitoring/perfdata"
	"nocternity.net/go/monitoring/plugin"

	"github.com/karrick/golf"
)

//--------------------------------------------------------------------------------------------------------

// Interface that can be implemented to fetch TLS certificates.
type certGetter interface {
	getCertificate(tlsConfig *tls.Config, address string) (*x509.Certificate, error)
}

// Full TLS certificate fetcher
type fullTLSGetter struct{}

func (f fullTLSGetter) getCertificate(tlsConfig *tls.Config, address string) (*x509.Certificate, error) {
	conn, err := tls.Dial("tcp", address, tlsConfig)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if err := conn.Handshake(); err != nil {
		return nil, err
	}
	return conn.ConnectionState().PeerCertificates[0], nil
}

// SMTP+STARTTLS certificate getter
type smtpGetter struct{}

func (f smtpGetter) cmd(tcon *textproto.Conn, expectCode int, text string) (int, string, error) {
	id, err := tcon.Cmd("%s", text)
	if err != nil {
		return 0, "", err
	}
	tcon.StartResponse(id)
	defer tcon.EndResponse(id)
	return tcon.ReadResponse(expectCode)
}

func (f smtpGetter) getCertificate(tlsConfig *tls.Config, address string) (*x509.Certificate, error) {
	conn, err := net.Dial("tcp", address)
	if err != nil {
		return nil, err
	}
	text := textproto.NewConn(conn)
	defer text.Close()
	if _, _, err := text.ReadResponse(220); err != nil {
		return nil, err
	}
	if _, _, err := f.cmd(text, 250, "HELO localhost"); err != nil {
		return nil, err
	}
	if _, _, err := f.cmd(text, 220, "STARTTLS"); err != nil {
		return nil, err
	}
	t := tls.Client(conn, tlsConfig)
	if err := t.Handshake(); err != nil {
		return nil, err
	}
	return t.ConnectionState().PeerCertificates[0], nil
}

// ManageSieve STARTTLS certificate getter
type sieveGetter struct{}

func (f sieveGetter) waitOK(conn net.Conn) error {
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "OK") {
			return nil
		}
		if strings.HasPrefix(line, "NO ") {
			return errors.New(line[3:])
		}
		if strings.HasPrefix(line, "BYE ") {
			return errors.New(line[4:])
		}
	}
	return scanner.Err()
}

func (f sieveGetter) runCmd(conn net.Conn, cmd string) error {
	if _, err := fmt.Fprintf(conn, "%s\r\n", cmd); err != nil {
		return err
	}
	return f.waitOK(conn)
}

func (f sieveGetter) getCertificate(tlsConfig *tls.Config, address string) (*x509.Certificate, error) {
	conn, err := net.Dial("tcp", address)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if err := f.waitOK(conn); err != nil {
		return nil, err
	}
	if err := f.runCmd(conn, "STARTTLS"); err != nil {
		return nil, err
	}
	t := tls.Client(conn, tlsConfig)
	if err := t.Handshake(); err != nil {
		return nil, err
	}
	return t.ConnectionState().PeerCertificates[0], nil
}

// Supported StartTLS protocols
var certGetters map[string]certGetter = map[string]certGetter{
	"":      fullTLSGetter{},
	"smtp":  &smtpGetter{},
	"sieve": &sieveGetter{},
}

// Get a string that represents supported StartTLS protocols
func listSupportedGetters() string {
	sb := strings.Builder{}
	for key := range certGetters {
		if sb.Len() != 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(key)
	}
	return sb.String()
}

//--------------------------------------------------------------------------------------------------------

// Command line flags that have been parsed.
type programFlags struct {
	hostname     string   // Main host name to connect to
	port         int      // TCP port to connect to
	warn         int      // Threshold for warning state (days)
	crit         int      // Threshold for critical state (days)
	ignoreCnOnly bool     // Do not warn about SAN-less certificates
	extraNames   []string // Extra names the certificate should include
	startTLS     string   // Protocol to use before requesting a switch to TLS.
}

// Program data including configuration and runtime data.
type checkProgram struct {
	programFlags                   // Flags from the command line
	plugin       *plugin.Plugin    // Plugin output state
	getter       certGetter        // Certificate getter
	certificate  *x509.Certificate // X.509 certificate from the server
}

// Parse command line arguments and store their values. If the -h flag is present,
// help will be displayed and the program will exit.
func (flags *programFlags) parseArguments() {
	var (
		names string
		help  bool
	)
	golf.BoolVarP(&help, 'h', "help", false, "Display usage information")
	golf.StringVarP(&flags.hostname, 'H', "hostname", "", "Host name to connect to.")
	golf.IntVarP(&flags.port, 'P', "port", -1, "Port to connect to.")
	golf.IntVarP(&flags.warn, 'W', "warning", -1,
		"Validity threshold below which a warning state is issued, in days.")
	golf.IntVarP(&flags.crit, 'C', "critical", -1,
		"Validity threshold below which a critical state is issued, in days.")
	golf.BoolVar(&flags.ignoreCnOnly, "ignore-cn-only", false,
		"Do not issue warnings regarding certificates that do not use SANs at all.")
	golf.StringVarP(&names, 'a', "additional-names", "",
		"A comma-separated list of names that the certificate should also provide.")
	golf.StringVarP(&flags.startTLS, 's', "start-tls", "",
		fmt.Sprintf(
			"Protocol to use before requesting a switch to TLS. "+
				"Supported protocols: %s.",
			listSupportedGetters()))
	golf.Parse()
	if help {
		golf.Usage()
		os.Exit(0)
	}
	if names == "" {
		flags.extraNames = make([]string, 0)
	} else {
		flags.extraNames = strings.Split(names, ",")
	}
}

// Initialise the monitoring check program.
func newProgram() *checkProgram {
	program := &checkProgram{
		plugin: plugin.New("Certificate check"),
	}
	program.parseArguments()
	return program
}

// Terminate the monitoring check program.
func (program *checkProgram) close() {
	program.plugin.Done()
}

// Check the values that were specified from the command line. Returns true
// if the arguments made sense.
func (program *checkProgram) checkFlags() bool {
	if program.hostname == "" {
		program.plugin.SetState(plugin.UNKNOWN, "no hostname specified")
		return false
	}
	if program.port < 1 || program.port > 65535 {
		program.plugin.SetState(plugin.UNKNOWN, "invalid or missing port number")
		return false
	}
	if program.warn != -1 && program.crit != -1 && program.warn <= program.crit {
		program.plugin.SetState(plugin.UNKNOWN, "nonsensical thresholds")
		return false
	}
	if _, ok := certGetters[program.startTLS]; !ok {
		errstr := fmt.Sprintf("unsupported StartTLS protocol %s", program.startTLS)
		program.plugin.SetState(plugin.UNKNOWN, errstr)
		return false
	}
	program.hostname = strings.ToLower(program.hostname)
	return true
}

// Connect to the remote host and obtain the certificate. Returns an error
// if connecting or performing the TLS handshake fail.
func (program *checkProgram) getCertificate() error {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS10,
	}
	connString := fmt.Sprintf("%s:%d", program.hostname, program.port)
	certificate, err := certGetters[program.startTLS].getCertificate(tlsConfig, connString)
	program.certificate = certificate
	return err
}

// Check that the CN of a certificate that doesn't contain a SAN actually
// matches the requested host name.
func (program *checkProgram) checkSANlessCertificate() bool {
	if !program.ignoreCnOnly || len(program.extraNames) != 0 {
		program.plugin.SetState(plugin.WARNING,
			"certificate doesn't have SAN domain names")
		return false
	}
	dn := strings.ToLower(program.certificate.Subject.String())
	if !strings.HasPrefix(dn, fmt.Sprintf("cn=%s,", program.hostname)) {
		program.plugin.SetState(plugin.CRITICAL, "incorrect certificate CN")
		return false
	}
	return true
}

// Checks whether a name is listed in the certificate's DNS names. If the name
// cannot be found, a line will be added to the plugin output and false will
// be returned.
func (program *checkProgram) checkHostName(name string) bool {
	for _, n := range program.certificate.DNSNames {
		if strings.ToLower(n) == name {
			return true
		}
	}
	program.plugin.AddLine(fmt.Sprintf("missing DNS name %s in certificate", name))
	return false
}

// Ensure the certificate matches the specified names. Returns false if it
// doesn't.
func (program *checkProgram) checkNames() bool {
	if len(program.certificate.DNSNames) == 0 {
		return program.checkSANlessCertificate()
	}
	ok := program.checkHostName(program.hostname)
	for _, name := range program.extraNames {
		ok = program.checkHostName(name) && ok
	}
	if !ok {
		program.plugin.SetState(plugin.CRITICAL, "names missing from SAN domain names")
	}
	return ok
}

// Check a certificate's time to expiry agains the warning and critical
// thresholds, returning a status code and description based on these
// values.
func (program *checkProgram) checkCertificateExpiry(tlDays int) (plugin.Status, string) {
	if tlDays <= 0 {
		return plugin.CRITICAL, "certificate expired"
	}
	var limitStr string
	var state plugin.Status
	if program.crit > 0 && tlDays <= program.crit {
		limitStr = fmt.Sprintf(" (<= %d)", program.crit)
		state = plugin.CRITICAL
	} else if program.warn > 0 && tlDays <= program.warn {
		limitStr = fmt.Sprintf(" (<= %d)", program.warn)
		state = plugin.WARNING
	} else {
		limitStr = ""
		state = plugin.OK
	}
	statusString := fmt.Sprintf("certificate will expire in %d days%s",
		tlDays, limitStr)
	return state, statusString
}

// Set the plugin's performance data based on the time left before the
// certificate expires and the thresholds.
func (program *checkProgram) setPerfData(tlDays int) {
	pdat := perfdata.New("validity", perfdata.UOM_NONE, fmt.Sprintf("%d", tlDays))
	if program.crit > 0 {
		pdat.SetCrit(perfdata.PDRMax(fmt.Sprint(program.crit)))
	}
	if program.warn > 0 {
		pdat.SetWarn(perfdata.PDRMax(fmt.Sprint(program.warn)))
	}
	program.plugin.AddPerfData(pdat)
}

// Run the check: fetch the certificate, check its names then check its time
// to expiry and update the plugin's performance data.
func (program *checkProgram) runCheck() {
	err := program.getCertificate()
	if err != nil {
		program.plugin.SetState(plugin.UNKNOWN, err.Error())
	} else if program.checkNames() {
		timeLeft := program.certificate.NotAfter.Sub(time.Now())
		tlDays := int((timeLeft + 86399*time.Second) / (24 * time.Hour))
		program.plugin.SetState(program.checkCertificateExpiry(tlDays))
		program.setPerfData(tlDays)
	}
}

func main() {
	program := newProgram()
	defer program.close()
	if program.checkFlags() {
		program.runCheck()
	}
}
