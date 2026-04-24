package resolve

import "fmt"

const (
	OutputTable = "table"
	OutputJSON  = "json"
	OutputYAML  = "yaml"
)

// ValidateOutput enforces FR-04 / EC-B. The prefix "output:" is reserved
// exclusively for this violation (NFR-03).
func ValidateOutput(v string) error {
	switch v {
	case OutputTable, OutputJSON, OutputYAML:
		return nil
	default:
		return fmt.Errorf("output: invalid value %q (allowed: table,json,yaml)", v)
	}
}
