package plugin

import (
	"container/list"
	"fmt"
	"nocternity.net/monitoring/perfdata"
	"os"
	"strings"
)

type Status int

const (
	OK Status = iota
	WARNING
	ERROR
	UNKNOWN
)

func (s Status) String() string {
	return [...]string{"OK", "WARNING", "ERROR", "UNKNOWN"}[s]
}

type Plugin struct {
	name      string
	status    Status
	message   string
	extraText *list.List
	perfData  map[string]perfdata.PerfData
}

func New(name string) *Plugin {
	p := new(Plugin)
	p.name = name
	p.status = UNKNOWN
	p.message = "no status set"
	p.perfData = make(map[string]perfdata.PerfData)
	return p
}

func (p *Plugin) SetState(status Status, message string) {
	p.status = status
	p.message = message
	p.extraText = nil
}

func (p *Plugin) AddLine(line string) {
	if p.extraText == nil {
		p.extraText = list.New()
	}
	p.extraText.PushBack(line)
}

func (p *Plugin) AddLines(lines []string) {
	for _, line := range lines {
		p.AddLine(line)
	}
}

func (p *Plugin) AddPerfData(pd perfdata.PerfData) {
	_, exists := p.perfData[pd.Label]
	if exists {
		panic(fmt.Sprintf("duplicate performance data %s", pd.Label))
	}
	p.perfData[pd.Label] = pd
}

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
