package render

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"

	feediumapi "github.com/4itosik/feedium/api/feedium"
)

const feedTableHeader = "POST_ID | SOURCE_ID | SCORE | PUBLISHED_AT | AUTHOR"

func writeFeedListTable(w io.Writer, resp *feediumapi.V1ListFeedResponse) error {
	if _, err := fmt.Fprintln(w, feedTableHeader); err != nil {
		return err
	}
	for _, item := range resp.GetItems() {
		author := item.GetAuthor()
		if author == "" {
			author = "-"
		}
		if _, err := fmt.Fprintf(w, "%s | %s | %s | %s | %s\n",
			item.GetPostId(),
			item.GetSourceId(),
			strconv.FormatFloat(item.GetScore(), 'f', -1, 64),
			formatTimestamp(item.GetPublishedAt()),
			author,
		); err != nil {
			return err
		}
	}
	return nil
}

func writeFeedListJSON(w io.Writer, resp *feediumapi.V1ListFeedResponse) error {
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

func writeFeedListYAML(w io.Writer, resp *feediumapi.V1ListFeedResponse) error {
	items := resp.GetItems()
	itemsIface := make([]proto.Message, len(items))
	for i, item := range items {
		itemsIface[i] = item
	}
	return writeItemsAsYAML(w, itemsIface)
}
