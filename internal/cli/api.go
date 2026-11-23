package cli

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

// readPOST lists path segments of endpoints that take POST but only read data.
var readPOST = map[string]bool{
	"_search": true, "_msearch": true, "_count": true, "_query": true,
	"_field_caps": true, "_explain": true, "_validate": true, "_sql": true,
	"_eql": true, "_mget": true, "_termvectors": true, "_mtermvectors": true,
	"_terms_enum": true, "_knn_search": true, "_rank_eval": true, "_async_search": true,
}

// writeSegments lists path segments that write documents. A document ID such
// as "_search" after one of them must not make the request look read-only.
var writeSegments = map[string]bool{
	"_doc": true, "_create": true, "_update": true, "_bulk": true,
}

// isReadOnly reports whether a request only reads data. GET and HEAD always
// read; POST reads when the path names a search or query endpoint.
func isReadOnly(method, path string) bool {
	switch method {
	case http.MethodGet, http.MethodHead:
		return true
	case http.MethodPost:
		read := false
		for _, seg := range strings.Split(strings.Trim(path, "/"), "/") {
			if writeSegments[seg] {
				return false
			}
			read = read || readPOST[seg]
		}
		return read
	default:
		return false
	}
}

func (a *app) apiCmd() *cobra.Command {
	var body string
	var allowWrite bool
	cmd := &cobra.Command{
		Use:   "api <method> <path>",
		Short: "Send any request to the Elasticsearch REST API",
		Long: `Send a request to any Elasticsearch REST API endpoint and print the response.

escli is read-only by default: it sends GET and HEAD requests, and POST
requests to search and query endpoints such as _search, _count and _query.
Any other request needs --allow-write. The real protection is a read-only API
key; this check prevents mistakes.`,
		Example: `  escli api GET /_cluster/health
  escli api POST /deals/_search --body '{"size":0,"aggs":{"stages":{"terms":{"field":"stage"}}}}'
  escli api GET '/_cat/indices?v&s=index' `,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			method := strings.ToUpper(args[0])
			target, err := url.Parse(args[1])
			if err != nil || !strings.HasPrefix(args[1], "/") || target.Host != "" {
				return fmt.Errorf("the path %q must start with '/', for example /_cluster/health", args[1])
			}
			if !allowWrite && !isReadOnly(method, target.Path) {
				return fmt.Errorf("%s %s may change data; escli is read-only unless you pass --allow-write", method, target.Path)
			}
			var req []byte
			if body != "" {
				if req, err = a.readBody(body); err != nil {
					return err
				}
			}
			c, err := a.connect()
			if err != nil {
				return err
			}
			resp, err := c.Do(cmd.Context(), method, target.EscapedPath(), target.Query(), req)
			if err != nil {
				return err
			}
			return a.writeBody(resp)
		},
	}
	cmd.Flags().StringVar(&body, "body", "", `request body as JSON, "@file" or "-" for stdin`)
	cmd.Flags().BoolVar(&allowWrite, "allow-write", false, "allow requests that may change data")
	return cmd
}
