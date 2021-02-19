// Package plugin implements a helper that can be used to implement a Nagios,
// Centreon, Icinga... service monitoring plugin.
package plugin

import (
	"container/list"
	"fmt"
	"os"
	"strings"

	"nocternity.net/monitoring/perfdata"
)

// Status represents the return status of the monitoring plugin. The
// corresponding integer value will be used as the program's exit code,
// to be interpreted by the monitoring system.
type Status int

// Plugin exit statuses.
const (
	OK Status = iota
	WARNING
	CRITICAL
	UNKNOWN
)

// String representations of the plugin statuses.
func (s Status) String() string {
	return [...]string{"OK", "WARNING", "ERROR", "UNKNOWN"}[s]
}

// Plugin represents the monitoring plugin's state, including its name,
// return status and message, additional lines of text, and performance
// data to be encoded in the output.
type Plugin struct {
	name      string
	status    Status
	message   string
	extraText *list.List
	perfData  map[string]*perfdata.PerfData
}

// New creates the plugin with `name` as its name and an unknown status.
func New(name string) *Plugin {
	p := new(Plugin)
	p.name = name
	p.status = UNKNOWN
	p.message = "no status set"
	p.perfData = make(map[string]*perfdata.PerfData)
	return p
}

// SetState sets the plugin's output code to `status` and its message to
// the specified `message`.
func (p *Plugin) SetState(status Status, message string) {
	p.status = status
	p.message = message
}

// AddLine adds the specified string to the extra output text buffer.
func (p *Plugin) AddLine(line string) {
	if p.extraText == nil {
		p.extraText = list.New()
	}
	p.extraText.PushBack(line)
}

// AddLines add the specified `lines` to the output text.
func (p *Plugin) AddLines(lines []string) {
	for _, line := range lines {
		p.AddLine(line)
	}
}

// AddPerfData adds performance data described by the "pd" argument to the
// output's performance data. If two performance data records are added for
// the same label, the program panics.
func (p *Plugin) AddPerfData(pd *perfdata.PerfData) {
	_, exists := p.perfData[pd.Label]
	if exists {
		panic(fmt.Sprintf("duplicate performance data %s", pd.Label))
	}
	p.perfData[pd.Label] = pd
}

// Done generates the plugin's text output from its name, status, text data
// and performance data, before exiting with the code corresponding to the
// status.
func (p *Plugin) Done() {
	var sb strings.Builder
	sb.WriteString(p.name)
	sb.WriteString(" ")
	sb.WriteString(p.status.String())
	sb.WriteString(": ")
	sb.WriteString(p.message)
	if len(p.perfData) > 0 {
		sb.WriteString(" | ")
		needSep := false
		for k := range p.perfData {
			if needSep {
				sb.WriteString(", ")
			} else {
				needSep = true
			}
			sb.WriteString(p.perfData[k].String())
		}
	}
	if p.extraText != nil {
		for em := p.extraText.Front(); em != nil; em = em.Next() {
			sb.WriteString("\n")
			sb.WriteString(em.Value.(string))
		}
	}
	fmt.Println(sb.String())
	os.Exit(int(p.status))
}
