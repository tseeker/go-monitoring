package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strings"
	"time"

	"nocternity.net/monitoring/perfdata"
	"nocternity.net/monitoring/plugin"

	"github.com/karrick/golf"
)

type programFlags struct {
	hostname     string
	port         int
	warn         int
	crit         int
	ignoreCnOnly bool
}

type checkProgram struct {
	programFlags
	plugin      *plugin.Plugin
	certificate *x509.Certificate
}

func (flags *programFlags) parseArguments() {
	var help bool
	golf.BoolVarP(&help, 'h', "help", false, "Display usage information")
	golf.StringVarP(&flags.hostname, 'H', "hostname", "", "Host name to connect to.")
	golf.IntVarP(&flags.port, 'P', "port", -1, "Port to connect to.")
	golf.IntVarP(&flags.warn, 'W', "warning", -1,
		"Validity threshold below which a warning state is issued, in days.")
	golf.IntVarP(&flags.crit, 'C', "critical", -1,
		"Validity threshold below which a critical state is issued, in days.")
	golf.BoolVar(&flags.ignoreCnOnly, "ignore-cn-only", false,
		"Do not issue warnings regarding certificates that do not use SANs at all.")
	golf.Parse()
	if help {
		golf.Usage()
		os.Exit(0)
	}
}

func newProgram() *checkProgram {
	program := &checkProgram{
		plugin: plugin.New("Certificate check"),
	}
	program.parseArguments()
	return program
}

func (program *checkProgram) close() {
	program.plugin.Done()
}

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
	program.hostname = strings.ToLower(program.hostname)
	return true
}

func (program *checkProgram) getCertificate() error {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS10,
	}
	connString := fmt.Sprintf("%s:%d", program.hostname, program.port)
	conn, err := tls.Dial("tcp", connString, tlsConfig)
	if err != nil {
		return fmt.Errorf("connection failed: %s", err.Error())
	}
	defer conn.Close()
	if err := conn.Handshake(); err != nil {
		return fmt.Errorf("handshake failed: %s", err.Error())
	}
	program.certificate = conn.ConnectionState().PeerCertificates[0]
	return nil
}

func findHostname(cert *x509.Certificate, hostname string) bool {
	for _, name := range cert.DNSNames {
		if strings.ToLower(name) == hostname {
			return true
		}
	}
	return false
}

func (program *checkProgram) checkCertificateName() bool {
	if len(program.certificate.DNSNames) == 0 {
		if !program.ignoreCnOnly {
			program.plugin.SetState(plugin.WARNING,
				"certificate doesn't have SAN domain names")
			return false
		}
		dn := strings.ToLower(program.certificate.Subject.String())
		if !strings.HasPrefix(dn, fmt.Sprintf("cn=%s,", program.hostname)) {
			program.plugin.SetState(plugin.CRITICAL, "incorrect certificate CN")
			return false
		}
	} else if !findHostname(program.certificate, program.hostname) {
		program.plugin.SetState(plugin.CRITICAL, "host name not found in SAN domain names")
		return false
	}
	return true
}

func (program *checkProgram) checkCertificateExpiry(tlDays int) (plugin.Status, string) {
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

func (program *checkProgram) runCheck() {
	err := program.getCertificate()
	if err != nil {
		program.plugin.SetState(plugin.UNKNOWN, err.Error())
	} else if program.checkCertificateName() {
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
