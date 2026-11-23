package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

func (a *app) infoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info",
		Short: "Show the cluster name and version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := a.connect()
			if err != nil {
				return err
			}
			body, err := c.Do(cmd.Context(), http.MethodGet, "/", nil, nil)
			if err != nil {
				return err
			}
			return a.writeBody(body)
		},
	}
}

func (a *app) indicesCmd() *cobra.Command {
	var all, stats bool
	cmd := &cobra.Command{
		Use:   "indices [pattern]",
		Short: "List indices, aliases and data streams",
		Long: `List the indices, aliases and data streams that match a pattern (default "*").

With --stats, list indices only, with document counts and sizes.`,
		Example: `  escli indices
  escli indices 'logs-*'
  escli indices --stats`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pattern := "*"
			if len(args) == 1 {
				pattern = args[0]
			}
			if err := checkIndex(pattern); err != nil {
				return err
			}
			c, err := a.connect()
			if err != nil {
				return err
			}
			q := url.Values{}
			if all {
				q.Set("expand_wildcards", "all")
			}
			path := "/_resolve/index/" + pattern
			if stats {
				path = "/_cat/indices/" + pattern
				q.Set("format", "json")
				q.Set("s", "index")
				q.Set("bytes", "b")
			}
			body, err := c.Do(cmd.Context(), http.MethodGet, path, q, nil)
			if err != nil {
				return err
			}
			return a.writeBody(body)
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "include hidden and system indices")
	cmd.Flags().BoolVar(&stats, "stats", false, "list indices with document counts and sizes")
	return cmd
}

func (a *app) mappingCmd() *cobra.Command {
	var flat bool
	cmd := &cobra.Command{
		Use:   "mapping <index>",
		Short: "Show the fields of an index",
		Long: `Show the mapping (field names and types) of an index or pattern.

With --flat, print one "field.path": "type" pair per field, which is shorter
and easier for an AI agent to use.`,
		Example: `  escli mapping deals --flat`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkIndex(args[0]); err != nil {
				return err
			}
			c, err := a.connect()
			if err != nil {
				return err
			}
			body, err := c.Do(cmd.Context(), http.MethodGet, "/"+args[0]+"/_mapping", nil, nil)
			if err != nil {
				return err
			}
			if !flat {
				return a.writeBody(body)
			}
			fields, err := flattenMappings(body)
			if err != nil {
				return err
			}
			return a.writeJSON(fields)
		},
	}
	cmd.Flags().BoolVar(&flat, "flat", false, "print field paths and types only")
	return cmd
}

type mappingField struct {
	Type       string                  `json:"type"`
	Properties map[string]mappingField `json:"properties"`
	Fields     map[string]mappingField `json:"fields"`
}

// flattenMappings turns a _mapping response into index -> field path -> type.
// Object fields are left out; their sub-fields are listed instead.
func flattenMappings(body []byte) (map[string]map[string]string, error) {
	var resp map[string]struct {
		Mappings struct {
			Properties map[string]mappingField `json:"properties"`
		} `json:"mappings"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("read the mapping: %w", err)
	}
	out := make(map[string]map[string]string, len(resp))
	for index, m := range resp {
		fields := map[string]string{}
		walkFields("", m.Mappings.Properties, fields)
		out[index] = fields
	}
	return out, nil
}

func walkFields(prefix string, props map[string]mappingField, out map[string]string) {
	for name, f := range props {
		path := name
		if prefix != "" {
			path = prefix + "." + name
		}
		if f.Type != "" && f.Type != "object" {
			out[path] = f.Type
		}
		for sub, sf := range f.Fields {
			out[path+"."+sub] = sf.Type
		}
		walkFields(path, f.Properties, out)
	}
}

type searchOptions struct {
	query  string
	body   string
	size   int
	fields []string
	sort   []string
	raw    bool
}

func (a *app) searchCmd() *cobra.Command {
	var o searchOptions
	cmd := &cobra.Command{
		Use:   "search <index>",
		Short: "Search documents",
		Long: `Search an index or pattern and print the total and the matching documents.

-q takes a Lucene query string, for example 'stage:won AND amount:>10000'.
--body takes a full search request as JSON, "@file" or "-" for stdin; the
other flags override the matching keys of the body. --raw prints the complete
Elasticsearch response, including hit metadata.`,
		Example: `  escli search deals -q 'stage:closedwon' --size 5 --fields name,amount
  escli search deals --sort amount:desc --size 3
  escli search deals --body '{"query":{"range":{"amount":{"gte":10000}}},"aggs":{"by_stage":{"terms":{"field":"stage"}}}}' --size 0`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkIndex(args[0]); err != nil {
				return err
			}
			req, err := a.searchBody(cmd, o)
			if err != nil {
				return err
			}
			c, err := a.connect()
			if err != nil {
				return err
			}
			body, err := c.Do(cmd.Context(), http.MethodPost, "/"+args[0]+"/_search", nil, req)
			if err != nil {
				return err
			}
			if o.raw {
				return a.writeBody(body)
			}
			summary, err := summarizeSearch(body)
			if err != nil {
				return err
			}
			return a.writeJSON(summary)
		},
	}
	f := cmd.Flags()
	f.StringVarP(&o.query, "query", "q", "", "Lucene query string")
	f.StringVar(&o.body, "body", "", `search request as JSON, "@file" or "-" for stdin`)
	f.IntVarP(&o.size, "size", "n", 10, "number of documents to return")
	f.StringSliceVar(&o.fields, "fields", nil, "only return these source fields (comma-separated, wildcards allowed)")
	f.StringSliceVar(&o.sort, "sort", nil, `sort order, for example "amount:desc,name"`)
	f.BoolVar(&o.raw, "raw", false, "print the complete Elasticsearch response")
	return cmd
}

// searchBody builds the search request from --body and the other flags.
func (a *app) searchBody(cmd *cobra.Command, o searchOptions) ([]byte, error) {
	req := map[string]any{}
	if o.body != "" {
		data, err := a.readBody(o.body)
		if err != nil {
			return nil, err
		}
		if err := decode(data, &req); err != nil {
			return nil, fmt.Errorf("the request body must be a JSON object: %w", err)
		}
	}
	if _, ok := req["size"]; !ok || cmd.Flags().Changed("size") {
		req["size"] = o.size
	}
	if o.query != "" {
		req["query"] = map[string]any{"query_string": map[string]any{"query": o.query}}
	}
	if len(o.fields) > 0 {
		req["_source"] = o.fields
	}
	if len(o.sort) > 0 {
		var sorts []any
		for _, s := range o.sort {
			field, order, hasOrder := strings.Cut(s, ":")
			if !hasOrder {
				sorts = append(sorts, field)
				continue
			}
			if order != "asc" && order != "desc" {
				return nil, fmt.Errorf("sort order in %q must be asc or desc", s)
			}
			sorts = append(sorts, map[string]any{field: map[string]any{"order": order}})
		}
		req["sort"] = sorts
	}
	if _, ok := req["track_total_hits"]; !ok {
		req["track_total_hits"] = true
	}
	return json.Marshal(req)
}

type searchSummary struct {
	Total         *int64            `json:"total,omitempty"`
	TotalRelation string            `json:"total_relation,omitempty"`
	Hits          []json.RawMessage `json:"hits"`
	Aggregations  json.RawMessage   `json:"aggregations,omitempty"`
}

// summarizeSearch keeps the total, the documents and the aggregations of a
// search response. Each document is its _source with _index and _id added.
func summarizeSearch(body []byte) (*searchSummary, error) {
	var resp struct {
		Hits struct {
			Total *struct {
				Value    int64  `json:"value"`
				Relation string `json:"relation"`
			} `json:"total"`
			Hits []struct {
				Index  string                     `json:"_index"`
				ID     string                     `json:"_id"`
				Source map[string]json.RawMessage `json:"_source"`
				Fields map[string]json.RawMessage `json:"fields"`
			} `json:"hits"`
		} `json:"hits"`
		Aggregations json.RawMessage `json:"aggregations"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("read the search response: %w", err)
	}
	s := &searchSummary{Hits: []json.RawMessage{}, Aggregations: resp.Aggregations}
	if t := resp.Hits.Total; t != nil {
		s.Total = &t.Value
		if t.Relation != "eq" {
			s.TotalRelation = t.Relation
		}
	}
	for _, h := range resp.Hits.Hits {
		doc, err := orderedObject(h.Index, h.ID, h.Source, h.Fields)
		if err != nil {
			return nil, err
		}
		s.Hits = append(s.Hits, doc)
	}
	return s, nil
}

// orderedObject writes _index and _id first, then the source fields sorted by name.
func orderedObject(index, id string, source, fields map[string]json.RawMessage) (json.RawMessage, error) {
	var buf bytes.Buffer
	buf.WriteString("{")
	write := func(k string, v []byte) {
		if buf.Len() > 1 {
			buf.WriteByte(',')
		}
		kb, _ := json.Marshal(k)
		buf.Write(kb)
		buf.WriteByte(':')
		buf.Write(v)
	}
	ib, _ := json.Marshal(index)
	idb, _ := json.Marshal(id)
	write("_index", ib)
	write("_id", idb)
	keys := make([]string, 0, len(source))
	for k := range source {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		write(k, source[k])
	}
	if len(fields) > 0 {
		fb, err := json.Marshal(fields)
		if err != nil {
			return nil, err
		}
		write("_fields", fb)
	}
	buf.WriteString("}")
	return buf.Bytes(), nil
}

func (a *app) countCmd() *cobra.Command {
	var query string
	cmd := &cobra.Command{
		Use:     "count <index>",
		Short:   "Count documents",
		Example: `  escli count deals -q 'stage:closedwon'`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkIndex(args[0]); err != nil {
				return err
			}
			c, err := a.connect()
			if err != nil {
				return err
			}
			var req []byte
			if query != "" {
				req, _ = json.Marshal(map[string]any{"query": map[string]any{"query_string": map[string]any{"query": query}}})
			}
			body, err := c.Do(cmd.Context(), http.MethodPost, "/"+args[0]+"/_count", nil, req)
			if err != nil {
				return err
			}
			var resp struct {
				Count json.Number `json:"count"`
			}
			if err := decode(body, &resp); err != nil {
				return err
			}
			return a.writeJSON(map[string]json.Number{"count": resp.Count})
		},
	}
	cmd.Flags().StringVarP(&query, "query", "q", "", "Lucene query string")
	return cmd
}

func (a *app) getCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <index> <id>",
		Short: "Get one document by ID",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkIndex(args[0]); err != nil {
				return err
			}
			c, err := a.connect()
			if err != nil {
				return err
			}
			body, err := c.Do(cmd.Context(), http.MethodGet, "/"+args[0]+"/_doc/"+url.PathEscape(args[1]), nil, nil)
			if err != nil {
				return err
			}
			return a.writeBody(body)
		},
	}
}

func (a *app) esqlCmd() *cobra.Command {
	var format string
	var raw bool
	cmd := &cobra.Command{
		Use:   "esql <query>",
		Short: "Run an ES|QL query",
		Long: `Run an ES|QL query and print the rows as JSON objects.

Pass "-" to read the query from stdin. --format csv, tsv or txt prints a table
instead. ES|QL returns at most 1000 rows unless the query sets LIMIT.`,
		Example: `  escli esql 'FROM deals | WHERE amount > 10000 | STATS total = SUM(amount) BY stage'
  escli esql 'FROM deals | KEEP name, amount | SORT amount DESC | LIMIT 5' --format csv`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := args[0]
			if query == "-" {
				data, err := io.ReadAll(a.in)
				if err != nil {
					return err
				}
				query = string(data)
			}
			if strings.TrimSpace(query) == "" {
				return fmt.Errorf("the ES|QL query is empty")
			}
			q := url.Values{}
			switch format {
			case "json":
			case "csv", "tsv", "txt":
				q.Set("format", format)
			default:
				return fmt.Errorf("--format must be json, csv, tsv or txt")
			}
			c, err := a.connect()
			if err != nil {
				return err
			}
			req, _ := json.Marshal(map[string]string{"query": query})
			body, err := c.Do(cmd.Context(), http.MethodPost, "/_query", q, req)
			if err != nil {
				return err
			}
			if format != "json" {
				_, err := a.out.Write(body)
				return err
			}
			if raw {
				return a.writeBody(body)
			}
			rows, err := esqlRows(body)
			if err != nil {
				return err
			}
			return a.writeBody(rows)
		},
	}
	cmd.Flags().StringVar(&format, "format", "json", "output format: json, csv, tsv or txt")
	cmd.Flags().BoolVar(&raw, "raw", false, "print the complete Elasticsearch response (columns and values)")
	return cmd
}

// esqlRows turns an ES|QL response of columns and value arrays into a JSON
// array with one object per row, keeping the column order.
func esqlRows(body []byte) ([]byte, error) {
	var resp struct {
		Columns []struct {
			Name string `json:"name"`
		} `json:"columns"`
		Values [][]json.RawMessage `json:"values"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("read the ES|QL response: %w", err)
	}
	var buf bytes.Buffer
	buf.WriteByte('[')
	for i, row := range resp.Values {
		if len(row) != len(resp.Columns) {
			return nil, fmt.Errorf("ES|QL row %d has %d values for %d columns", i, len(row), len(resp.Columns))
		}
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.WriteByte('{')
		for j, col := range resp.Columns {
			if j > 0 {
				buf.WriteByte(',')
			}
			name, _ := json.Marshal(col.Name)
			buf.Write(name)
			buf.WriteByte(':')
			buf.Write(row[j])
		}
		buf.WriteByte('}')
	}
	buf.WriteByte(']')
	return buf.Bytes(), nil
}
