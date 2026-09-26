package routes

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

// censusRoute mirrors one entry of e2e/route-census.json.
type censusRoute struct {
	Path       string `json:"path"`
	Auth       string `json:"auth"`
	Param      string `json:"param,omitempty"`
	SkipCensus string `json:"skipCensus,omitempty"`
}

type censusFile struct {
	Routes []censusRoute `json:"routes"`
}

// devToolsRoutePrefix is excluded from the equality check: it's only
// registered when APP_DEV_TOOLS=true && APP_ENV!=prod (see
// registerAdminDevToolsRoutes), so it isn't part of the router's route set
// in prod even though it's a literal .GET(...) call in routes.go.
const devToolsRoutePrefix = "/admin/dev-tools"

// TestRouteCensus_MatchesRegisteredGETRoutes parses routes.go with go/ast to
// extract every literal .GET("...") path (PocketBase's RouterGroup has no
// exported way to enumerate registered routes — only HasRoute(method, path)
// to check one at a time) and asserts it's a set-equal match against
// e2e/route-census.json. A GET route added to routes.go without a matching
// census entry fails here; a stale census entry with no matching route also
// fails here. e2e/tests/route-census.spec.ts is the consumer that actually
// visits every entry in the browser.
func TestRouteCensus_MatchesRegisteredGETRoutes(t *testing.T) {
	registered := parseRegisteredGETPaths(t, "routes.go")

	data, err := os.ReadFile(filepath.Join("..", "e2e", "route-census.json"))
	require.NoError(t, err, "e2e/route-census.json must exist and be readable")
	var census censusFile
	require.NoError(t, json.Unmarshal(data, &census))

	censused := make(map[string]bool, len(census.Routes))
	for _, r := range census.Routes {
		censused[r.Path] = true
	}

	for path := range registered {
		if path == devToolsRoutePrefix {
			continue
		}
		if !censused[path] {
			t.Errorf("routes.go registers GET %s with no matching entry in e2e/route-census.json — add one (or skipCensus with a reason if it genuinely isn't a page route)", path)
		}
	}
	for path := range censused {
		if !registered[path] {
			t.Errorf("e2e/route-census.json lists GET %s but routes.go no longer registers it — remove the stale entry", path)
		}
	}
}

// adminGroupPrefix is the prefix registerAdminRoutes binds its group to
// (se.Router.Group("/admin")) — kept as a named constant so
// funcTakesAdminGroup's assumption ("every RouterGroup param means this
// prefix") stays correct if it's ever read alongside a routes.go change.
const adminGroupPrefix = "/admin"

// parseRegisteredGETPaths walks routes.go's AST and collects the full path
// (prefix included) from every `.GET("...")` call, on both se.Router (no
// prefix) and the admin sub-group (g.GET(...), prefixed with adminGroupPrefix
// — detected by the enclosing function's first parameter type, since a
// RouterGroup carries its prefix at runtime, not in the AST). Wildcard
// segment names (e.g. {id}) are kept verbatim to match route-census.json's
// format, since routes.go and the census both use PocketBase's own
// placeholder syntax.
func parseRegisteredGETPaths(t *testing.T, filename string) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, nil, 0)
	require.NoError(t, err, "parse %s", filename)

	paths := make(map[string]bool)
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		prefix := ""
		if funcTakesAdminGroup(fn) {
			prefix = adminGroupPrefix
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "GET" || len(call.Args) == 0 {
				return true
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			path, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			paths[prefix+path] = true
			return true
		})
	}
	return paths
}

// funcTakesAdminGroup reports whether fn's first parameter is a
// *router.RouterGroup[...] — the shape every registerAdmin*Routes function
// in routes.go uses to receive the pre-prefixed "/admin" group.
func funcTakesAdminGroup(fn *ast.FuncDecl) bool {
	if fn.Type.Params == nil || len(fn.Type.Params.List) == 0 {
		return false
	}
	star, ok := fn.Type.Params.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	index, ok := star.X.(*ast.IndexExpr)
	if !ok {
		return false
	}
	sel, ok := index.X.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	return sel.Sel.Name == "RouterGroup"
}
