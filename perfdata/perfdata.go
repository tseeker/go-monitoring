// Package `perfdata` provides representations for a monitoring plugin's
// performance data.
package perfdata

import (
	"fmt"
	"regexp"
	"strings"
)

// Units of measurement, which may be used to qualify the performance data.
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

// Flags indicating which elements of performance data have been set.
type perfDataBits int

const (
	PDAT_WARN perfDataBits = 1 << iota
	PDAT_CRIT
	PDAT_MIN
	PDAT_MAX
)

// Regexps used to check values and ranges in performance data records.
var (
	// Common value check regexp
	vcRegexp = `^-?(0(\.\d*)?|[1-9]\d*(\.\d*)?|\.\d+)$`
	// Compiled value check regexp
	valueCheck = regexp.MustCompile(vcRegexp)
	// Compiled range min value check
	rangeMinCheck = regexp.MustCompile(vcRegexp + `|^~$`)
)

// Performance data range
type PerfDataRange struct {
	start  string
	end    string
	inside bool
}

// Creates a performance data range from -inf to 0 and from the specified
// value to +inf.
func PDRMax(max string) *PerfDataRange {
	if !valueCheck.MatchString(max) {
		panic("invalid performance data range maximum value")
	}
	r := &PerfDataRange{}
	r.start = "0"
	r.end = max
	return r
}

// Creates a performance data range from -inf to the specified minimal value
// and from the specified maximal value to +inf.
func PDRMinMax(min, max string) *PerfDataRange {
	if !valueCheck.MatchString(max) {
		panic("invalid performance data range maximum value")
	}
	if !rangeMinCheck.MatchString(min) {
		panic("invalid performance data range minimum value")
	}
	r := &PerfDataRange{}
	r.start = min
	r.end = max
	return r
}

// Inverts the range.
func (r *PerfDataRange) Inside() *PerfDataRange {
	r.inside = true
	return r
}

// Generates the range's string representation so it can be sent to the
// monitoring system.
func (r *PerfDataRange) String() string {
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

// Performance data, including a label, units, a value, warning/critical
// ranges and min/max boundaries.
type PerfData struct {
	Label      string
	units      UnitOfMeasurement
	bits       perfDataBits
	value      string
	warn, crit PerfDataRange
	min, max   string
}

// Create performance data using the specified label and units.
func New(label string, units UnitOfMeasurement, value string) *PerfData {
	if value != "" && !valueCheck.MatchString(value) {
		panic("invalid value")
	}
	r := &PerfData{}
	r.Label = label
	r.units = units
	if value == "" {
		r.value = "U"
	} else {
		r.value = value
	}
	return r
}

// Set the warning range for the performance data record.
func (d *PerfData) SetWarn(r *PerfDataRange) {
	d.warn = *r
	d.bits = d.bits | PDAT_WARN
}

// Set the critical range for the performance data record.
func (d *PerfData) SetCrit(r *PerfDataRange) {
	d.crit = *r
	d.bits = d.bits | PDAT_CRIT
}

// Set the performance data's minimal value
func (d *PerfData) SetMin(min string) {
	if !valueCheck.MatchString(min) {
		panic("invalid value")
	}
	d.min = min
	d.bits = d.bits | PDAT_MIN
}

// Set the performance data's maximal value.
func (d *PerfData) SetMax(max string) {
	if !valueCheck.MatchString(max) {
		panic("invalid value")
	}
	d.max = max
	d.bits = d.bits | PDAT_MAX
}

// Converts performance data to a string which may be read by the monitoring
// system.
func (d *PerfData) String() string {
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
