package main

import (
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"strings"
	"time"

	"nocternity.net/monitoring/perfdata"
	"nocternity.net/monitoring/plugin"
)

type cliFlags struct {
	hostname     string
	port         int
	warn         int
	crit         int
	ignoreCnOnly bool
}

func handleCli(p *plugin.Plugin) (flags *cliFlags) {
	flags = &cliFlags{}
	flag.StringVar(&flags.hostname, "hostname", "", "Host name to connect to.")
	flag.StringVar(&flags.hostname, "H", "", "Host name to connect to (shorthand).")
	flag.IntVar(&flags.port, "port", -1, "Port to connect to.")
	flag.IntVar(&flags.port, "P", -1, "Port to connect to (shorthand).")
	flag.IntVar(&flags.warn, "warning", -1, "Validity threshold below which a warning state is issued, in days.")
	flag.IntVar(&flags.warn, "W", -1, "Validity threshold below which a warning state is issued, in days (shorthand).")
	flag.IntVar(&flags.crit, "critical", -1, "Validity threshold below which a critical state is issued, in days.")
	flag.IntVar(&flags.crit, "C", -1, "Validity threshold below which a critical state is issued, in days (shorthand).")
	flag.BoolVar(&flags.ignoreCnOnly, "ignore-cn-only", false,
		"Do not issue warnings regarding certificates that do not use SANs at all.")
	flag.Parse()
	return
}

func checkFlags(p *plugin.Plugin, flags *cliFlags) bool {
	if flags.hostname == "" {
		p.SetState(plugin.UNKNOWN, "no hostname specified")
		return false
	}
	if flags.port < 1 || flags.port > 65535 {
		p.SetState(plugin.UNKNOWN, "invalid or missing port number")
		return false
	}
	if flags.warn != -1 && flags.crit != -1 && flags.warn <= flags.crit {
		p.SetState(plugin.UNKNOWN, "nonsensical thresholds")
		return false
	}
	flags.hostname = strings.ToLower(flags.hostname)
	return true
}

func findHostname(cert *x509.Certificate, hostname string) bool {
	for _, name := range cert.DNSNames {
		if strings.ToLower(name) == hostname {
			return true
		}
	}
	return false
}

func main() {
	p := plugin.New("Certificate check")
	defer p.Done()
	flags := handleCli(p)
	if !checkFlags(p, flags) {
		return
	}

	tls_cfg := &tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS10,
	}
	tls_conn, tls_err := tls.Dial("tcp", fmt.Sprintf("%s:%d", flags.hostname, flags.port), tls_cfg)
	if tls_err != nil {
		p.SetState(plugin.UNKNOWN, fmt.Sprintf("connection failed: %s", tls_err))
		return
	}
	defer tls_conn.Close()
	if hsk_err := tls_conn.Handshake(); hsk_err != nil {
		p.SetState(plugin.UNKNOWN, fmt.Sprintf("handshake failed: %s", hsk_err))
		return
	}
	certificate := tls_conn.ConnectionState().PeerCertificates[0]

	if len(certificate.DNSNames) == 0 {
		if !flags.ignoreCnOnly {
			p.SetState(plugin.WARNING, "certificate doesn't have SAN domain names")
			return
		}
		dn := strings.ToLower(certificate.Subject.String())
		if !strings.HasPrefix(dn, fmt.Sprintf("cn=%s,", flags.hostname)) {
			p.SetState(plugin.CRITICAL, "incorrect certificate CN")
			return
		}
	} else if !findHostname(certificate, flags.hostname) {
		p.SetState(plugin.CRITICAL, "host name not found in SAN domain names")
		return
	}
	timeLeft := certificate.NotAfter.Sub(time.Now())
	tlDays := int((timeLeft + 86399*time.Second) / (24 * time.Hour))
	if flags.crit > 0 && tlDays <= flags.crit {
		p.SetState(plugin.CRITICAL, fmt.Sprintf("certificate will expire in %d days (<= %d)", tlDays, flags.crit))
	} else if flags.warn > 0 && tlDays <= flags.warn {
		p.SetState(plugin.WARNING, fmt.Sprintf("certificate will expire in %d days (<= %d)", tlDays, flags.warn))
	} else {
		p.SetState(plugin.OK, fmt.Sprintf("certificate will expire in %d days", tlDays))
	}

	pdat := perfdata.New("validity", perfdata.UOM_NONE, fmt.Sprintf("%d", tlDays))
	if flags.crit > 0 {
		pdat.SetCrit(perfdata.PDRMax(fmt.Sprint(flags.crit)))
	}
	if flags.warn > 0 {
		pdat.SetWarn(perfdata.PDRMax(fmt.Sprint(flags.warn)))
	}
	p.AddPerfData(pdat)
}
