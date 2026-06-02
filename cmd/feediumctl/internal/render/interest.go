package render

import (
	"fmt"
	"io"
	"strings"

	"google.golang.org/protobuf/proto"

	feediumapi "github.com/4itosik/feedium/api/feedium"
)

const interestTableHeader = "ID | TOPIC | MODEL | CREATED_AT | UPDATED_AT"

func writeInterestListTable(w io.Writer, resp *feediumapi.V1ListInterestsResponse) error {
	if _, err := fmt.Fprintln(w, interestTableHeader); err != nil {
		return err
	}
	for _, in := range resp.GetItems() {
		if err := writeInterestItemRow(w, in); err != nil {
			return err
		}
	}
	return nil
}

func writeInterestSingleTable(w io.Writer, in *feediumapi.Interest) error {
	if in == nil {
		panic("render: unreachable nil Interest in response")
	}
	if _, err := fmt.Fprintln(w, interestTableHeader); err != nil {
		return err
	}
	return writeInterestItemRow(w, in)
}

func writeInterestItemRow(w io.Writer, in *feediumapi.Interest) error {
	_, err := fmt.Fprintf(w, "%s | %s | %s | %s | %s\n",
		in.GetId(),
		in.GetTopic(),
		in.GetModel(),
		formatTimestamp(in.GetCreatedAt()),
		formatTimestamp(in.GetUpdatedAt()),
	)
	return err
}

func writeInterestListJSON(w io.Writer, resp *feediumapi.V1ListInterestsResponse) error {
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

func writeInterestListYAML(w io.Writer, resp *feediumapi.V1ListInterestsResponse) error {
	items := resp.GetItems()
	itemsIface := make([]proto.Message, len(items))
	for i, item := range items {
		itemsIface[i] = item
	}
	return writeItemsAsYAML(w, itemsIface)
}
