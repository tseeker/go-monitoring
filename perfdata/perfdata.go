package perfdata

import (
	"fmt"
	"regexp"
	"strings"
)

type UnitOfMeasurement int

const (
	UOM_NONE UnitOfMeasurement = iota
	UOM_SECONDS
	UOM_PERCENT
	UOM_BYTES
	UOM_KILOBYTES
	UOM_MEGABYTES
	UOM_GIGABYTES
	UOM_TERABYTES
	UOM_COUNTER
)

func (u UnitOfMeasurement) String() string {
	return [...]string{"", "s", "%", "B", "KB", "MB", "GB", "TB", "c"}[u]
}

type PerfDataBits int

const (
	PDAT_WARN PerfDataBits = 1 << iota
	PDAT_CRIT
	PDAT_MIN
	PDAT_MAX
)

var (
	valueCheck    = regexp.MustCompile(`^-?(0(\.\d*)?|[1-9]\d*(\.\d*)?|\.\d+)$`)
	rangeMinCheck = regexp.MustCompile(`^-?(0(\.\d*)?|[1-9]\d*(\.\d*)?|\.\d+)$|^~$`)
)

type PerfDataRange struct {
	start  string
	end    string
	inside bool
}

func PDRMax(max string) *PerfDataRange {
	if !valueCheck.MatchString(max) {
		panic("invalid performance data range maximum value")
	}
	r := new(PerfDataRange)
	r.start = "0"
	r.end = max
	return r
}

func PDRMinMax(min, max string) *PerfDataRange {
	if !valueCheck.MatchString(max) {
		panic("invalid performance data range maximum value")
	}
	if !rangeMinCheck.MatchString(min) {
		panic("invalid performance data range minimum value")
	}
	r := new(PerfDataRange)
	r.start = min
	r.end = max
	return r
}

func (r *PerfDataRange) Inside() *PerfDataRange {
	r.inside = true
	return r
}

func (r PerfDataRange) String() string {
	var start, inside string
	if r.start == "" {
		start = "~"
	} else if r.start == "0" {
		start = ""
	} else {
		start = r.start
	}
	if r.inside {
		inside = "@"
	} else {
		inside = ""
	}
	return fmt.Sprintf("%s%s:%s", inside, start, r.end)
}

type PerfData struct {
	Label      string
	units      UnitOfMeasurement
	bits       PerfDataBits
	value      string
	warn, crit PerfDataRange
	min, max   string
}

func New(label string, units UnitOfMeasurement, value string) PerfData {
	if value != "" && !valueCheck.MatchString(value) {
		panic("invalid value")
	}
	r := PerfData{}
	r.Label = label
	r.units = units
	if value == "" {
		r.value = "U"
	} else {
		r.value = value
	}
	return r
}

func (d *PerfData) SetWarn(r *PerfDataRange) {
	d.warn = *r
	d.bits = d.bits | PDAT_WARN
}

func (d *PerfData) SetCrit(r *PerfDataRange) {
	d.crit = *r
	d.bits = d.bits | PDAT_CRIT
}

func (d *PerfData) SetMin(min string) {
	if !valueCheck.MatchString(min) {
		panic("invalid value")
	}
	d.min = min
	d.bits = d.bits | PDAT_MIN
}

func (d *PerfData) SetMax(max string) {
	if !valueCheck.MatchString(max) {
		panic("invalid value")
	}
	d.max = max
	d.bits = d.bits | PDAT_MAX
}

func (d PerfData) String() string {
	var sb strings.Builder
	needsQuotes := strings.ContainsAny(d.Label, " '=\"")
	if needsQuotes {
		sb.WriteString("'")
	}
	sb.WriteString(strings.ReplaceAll(d.Label, "'", "''"))
	if needsQuotes {
		sb.WriteString("'")
	}
	sb.WriteString("=")
	sb.WriteString(fmt.Sprintf("%s%s;", d.value, d.units.String()))
	if d.bits&PDAT_WARN != 0 {
		sb.WriteString(d.warn.String())
	}
	sb.WriteString(";")
	if d.bits&PDAT_CRIT != 0 {
		sb.WriteString(d.crit.String())
	}
	sb.WriteString(";")
	if d.bits&PDAT_MIN != 0 {
		sb.WriteString(d.min)
	}
	sb.WriteString(";")
	if d.bits&PDAT_MAX != 0 {
		sb.WriteString(d.max)
	}

	return sb.String()
}
