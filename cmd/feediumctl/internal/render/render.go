// Package render serialises proto messages to the formats supported by
// feediumctl (FR-09, FR-10, INV-06).
package render

import (
	"fmt"
	"io"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"sigs.k8s.io/yaml"

	feediumapi "github.com/4itosik/feedium/api/feedium"
)

// Format identifies an output format. Use the constants below.
const (
	FormatTable = "table"
	FormatJSON  = "json"
	FormatYAML  = "yaml"
)

// jsonMarshaler is the single source of truth for protojson options (FR-10).
// Returned as a function so every call gets a fresh value and the renderer
// avoids shared mutable package-level state (INV-06).
func jsonMarshaler() protojson.MarshalOptions {
	return protojson.MarshalOptions{
		Multiline:       true,
		Indent:          "  ",
		UseProtoNames:   true,
		EmitUnpopulated: false,
	}
}

// listItemMarshaler is a compact (non-multiline) marshaler used for individual
// Source items inside the list JSON/YAML array. Key order and field filtering
// are identical to jsonMarshaler; only the multiline formatting differs.
func listItemMarshaler() protojson.MarshalOptions {
	return protojson.MarshalOptions{
		UseProtoNames:   true,
		EmitUnpopulated: false,
	}
}

// Write serialises msg to w in the requested format. The format is assumed
// valid (enforced upstream by resolve.ValidateOutput). Output always ends
// with exactly one trailing '\n'.
func Write(w io.Writer, format string, msg proto.Message) error {
	switch format {
	case FormatJSON:
		return writeJSON(w, msg)
	case FormatYAML:
		return writeYAML(w, msg)
	case FormatTable:
		return writeTable(w, msg)
	default:
		// Unreachable: resolve.ValidateOutput rejects any other value
		// before we reach the renderer. A panic flags the broken
		// invariant instead of leaking a non-NFR-03 prefix to stderr.
		panic(fmt.Sprintf("render: unreachable format %q", format))
	}
}

func writeJSON(w io.Writer, msg proto.Message) error {
	// source list outputs a JSON array of items, not the full response envelope.
	if resp, ok := msg.(*feediumapi.V1ListSourcesResponse); ok {
		return writeSourceListJSON(w, resp)
	}
	data, err := jsonMarshaler().Marshal(msg)
	if err != nil {
		return err
	}
	return writeExactlyOneTrailingNewline(w, data)
}

func writeYAML(w io.Writer, msg proto.Message) error {
	// source list outputs a YAML sequence, not the full response envelope.
	if resp, ok := msg.(*feediumapi.V1ListSourcesResponse); ok {
		return writeSourceListYAML(w, resp)
	}
	// Use a deterministic, non-multiline JSON representation as the input to
	// JSONToYAML. YAML key order is therefore driven by JSONToYAML itself,
	// which sorts keys lexicographically (INV-06).
	stable := protojson.MarshalOptions{
		UseProtoNames:   true,
		EmitUnpopulated: false,
	}
	data, err := stable.Marshal(msg)
	if err != nil {
		return err
	}
	out, err := yaml.JSONToYAML(data)
	if err != nil {
		return err
	}
	return writeExactlyOneTrailingNewline(w, out)
}

func writeTable(w io.Writer, msg proto.Message) error {
	switch m := msg.(type) {
	case *feediumapi.V1CheckResponse:
		return writeHealthTable(w, m)
	case *feediumapi.V1ListSourcesResponse:
		return writeSourceListTable(w, m)
	case *feediumapi.V1GetSourceResponse:
		return writeSourceSingleTable(w, m.GetSource())
	case *feediumapi.V1CreateSourceResponse:
		return writeSourceSingleTable(w, m.GetSource())
	case *feediumapi.V1UpdateSourceResponse:
		return writeSourceSingleTable(w, m.GetSource())
	default:
		// Unreachable: every proto message rendered by CLI is listed
		// above. Panicking preserves the NFR-03 prefix contract.
		panic(fmt.Sprintf("render: unreachable message type %T for table format", msg))
	}
}

func writeHealthTable(w io.Writer, resp *feediumapi.V1CheckResponse) error {
	_, err := fmt.Fprintf(w, "FIELD | VALUE\nstatus | %s\n", resp.GetStatus())
	return err
}

// writeSourceListJSON serialises the items slice as a JSON array (AC-S1, EC-C).
// Each item uses the compact listItemMarshaler for a flat, deterministic output.
func writeSourceListJSON(w io.Writer, resp *feediumapi.V1ListSourcesResponse) error {
	items := resp.GetItems()
	if len(items) == 0 {
		_, err := fmt.Fprintln(w, "[]")
		return err
	}
	parts := make([]string, len(items))
	for i, item := range items {
		data, err := listItemMarshaler().Marshal(item)
		if err != nil {
			return err
		}
		parts[i] = string(data)
	}
	_, err := fmt.Fprintf(w, "[%s]\n", strings.Join(parts, ","))
	return err
}

// writeSourceListYAML serialises the items slice as a YAML sequence (EC-C).
func writeSourceListYAML(w io.Writer, resp *feediumapi.V1ListSourcesResponse) error {
	items := resp.GetItems()
	if len(items) == 0 {
		_, err := fmt.Fprintln(w, "[]")
		return err
	}
	parts := make([]string, len(items))
	for i, item := range items {
		data, err := listItemMarshaler().Marshal(item)
		if err != nil {
			return err
		}
		parts[i] = string(data)
	}
	jsonArray := fmt.Sprintf("[%s]", strings.Join(parts, ","))
	out, err := yaml.JSONToYAML([]byte(jsonArray))
	if err != nil {
		return err
	}
	return writeExactlyOneTrailingNewline(w, out)
}

// WriteDelete renders a source-delete result (SR-05, AC-S4).
// It does not delegate to Write or protojson to guarantee byte-exact output
// independent of marshaler options (R-3 from handoff).
func WriteDelete(w io.Writer, format, id string) error {
	switch format {
	case FormatTable:
		_, err := fmt.Fprintf(w, "deleted: %s\n", id)
		return err
	case FormatJSON:
		_, err := fmt.Fprintf(w, "{\"deleted\":true,\"id\":\"%s\"}\n", id)
		return err
	case FormatYAML:
		// Keys sorted lexicographically: deleted, id (SR-05, SR-10).
		_, err := fmt.Fprintf(w, "deleted: true\nid: %s\n", id)
		return err
	default:
		// Unreachable: resolve.ValidateOutput guards this path.
		panic(fmt.Sprintf("render: unreachable format %q", format))
	}
}

// writeExactlyOneTrailingNewline writes data to w, ensuring the final byte
// sequence ends with a single '\n' (NFR-06).
func writeExactlyOneTrailingNewline(w io.Writer, data []byte) error {
	for len(data) > 1 && data[len(data)-1] == '\n' && data[len(data)-2] == '\n' {
		data = data[:len(data)-1]
	}
	if len(data) == 0 || data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}
	_, err := w.Write(data)
	return err
}
