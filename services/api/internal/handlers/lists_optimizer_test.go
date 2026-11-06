package handlers

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/LLRHook/swiss-grocery/services/api/internal/models"
	"github.com/LLRHook/swiss-grocery/services/api/internal/optimizer"
)

type offer struct {
	optimizer.Candidate
	productID int64
	currency  string
}

func priced(name string, storeID int, price float64) offer {
	return offer{
		Candidate: optimizer.Candidate{
			Name:  name,
			Store: models.Store{ID: storeID, Name: fmt.Sprintf("Store %d", storeID), Slug: fmt.Sprintf("store-%d", storeID)},
			Price: price,
		},
		productID: 1,
		currency:  "CHF",
	}
}

func candidates(offers []offer) []optimizer.Candidate {
	rows := make([]optimizer.Candidate, 0, len(offers))
	for _, offer := range offers {
		rows = append(rows, offer.Candidate)
	}
	return rows
}

// A stdlib database/sql driver supplies synthetic rows to the REAL Fiber
// handler. There is no optimizer interface, fake process or Go runtime oracle.
type fixture struct {
	rows        [][]driver.Value
	queryError  error
	streamError error
	closeError  error
	closed      atomic.Bool
	queried     atomic.Bool
	hasDeadline atomic.Bool
}

func fixtureFor(offers []offer) *fixture {
	f := &fixture{}
	for _, offer := range offers {
		f.rows = append(f.rows, []driver.Value{
			offer.Name, offer.productID, int64(offer.Store.ID), offer.Store.Name,
			offer.Store.Slug, offer.Price, offer.currency,
		})
	}
	return f
}

type fixtureConnector struct{ f *fixture }
type fixtureDriver struct{}
type fixtureConn struct{ f *fixture }
type fixtureRows struct {
	f     *fixture
	index int
}

func (c fixtureConnector) Connect(context.Context) (driver.Conn, error) {
	return &fixtureConn{f: c.f}, nil
}
func (fixtureConnector) Driver() driver.Driver         { return fixtureDriver{} }
func (fixtureDriver) Open(string) (driver.Conn, error) { return nil, errors.New("use connector") }
func (*fixtureConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}
func (*fixtureConn) Close() error              { return nil }
func (*fixtureConn) Begin() (driver.Tx, error) { return nil, errors.New("unexpected transaction") }

func (c *fixtureConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	const expected = `SELECT sli.name, sli.product_id, s.id, s.name, s.slug, sp.current_price, sp.currency
	 FROM shopping_list_items sli
	 JOIN store_products sp ON sp.product_id = sli.product_id
	 JOIN stores s ON s.id = sp.store_id
	 WHERE sli.list_id = $1 AND sli.product_id IS NOT NULL
	 ORDER BY sli.id, sp.current_price ASC`
	if strings.Join(strings.Fields(query), " ") != strings.Join(strings.Fields(expected), " ") ||
		len(args) != 1 || args[0].Value != "42" {
		return nil, errors.New("unexpected optimization query or bound list ID")
	}
	c.f.queried.Store(true)
	_, deadline := ctx.Deadline()
	c.f.hasDeadline.Store(deadline)
	if c.f.queryError != nil {
		return nil, c.f.queryError
	}
	return &fixtureRows{f: c.f}, nil
}

func (*fixtureRows) Columns() []string {
	return []string{"name", "product_id", "id", "store_name", "slug", "current_price", "currency"}
}
func (r *fixtureRows) Close() error { r.f.closed.Store(true); return r.f.closeError }
func (r *fixtureRows) Next(dest []driver.Value) error {
	if r.index == len(r.f.rows) {
		if r.f.streamError != nil {
			return r.f.streamError
		}
		return io.EOF
	}
	copy(dest, r.f.rows[r.index])
	r.index++
	return nil
}

func optimizeHTTP(t *testing.T, f *fixture) (int, []byte) {
	t.Helper()
	db := sql.OpenDB(fixtureConnector{f: f})
	t.Cleanup(func() { _ = db.Close() })
	app := fiber.New()
	handler := NewListHandler(db)
	app.Get("/api/v1/lists/:id/optimize", handler.Optimize)
	response, err := app.Test(httptest.NewRequest("GET", "/api/v1/lists/42/optimize", nil), fiber.TestConfig{
		Timeout:       5 * time.Second,
		FailOnTimeout: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !f.queried.Load() || !f.hasDeadline.Load() {
		t.Fatal("expected exact SQL through QueryContext with a deadline")
	}
	if f.queryError == nil && !f.closed.Load() {
		t.Fatal("SQL rows must be closed")
	}
	return response.StatusCode, body
}

func assertPlan(t *testing.T, offers []offer) models.OptimizedPlan {
	t.Helper()
	status, body := optimizeHTTP(t, fixtureFor(offers))
	if status != 200 {
		t.Fatalf("HTTP status %d: %s", status, body)
	}
	var got models.OptimizedPlan
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	expected := legacyReference(candidates(offers), true)
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("native HTTP plan differs from frozen reference\ngot: %#v\nwant: %#v", got, expected)
	}
	// DeepEqual treats signed zeros as equal; preserve Go rounding bits too.
	if math.Float64bits(got.TotalCost) != math.Float64bits(expected.TotalCost) ||
		math.Float64bits(got.Savings) != math.Float64bits(expected.Savings) {
		t.Fatal("total/savings bit parity differs (including signed zero)")
	}
	for i := range got.StorePlans {
		if math.Float64bits(got.StorePlans[i].Subtotal) != math.Float64bits(expected.StorePlans[i].Subtotal) {
			t.Fatal("subtotal bit parity differs")
		}
	}
	return got
}

func TestLegacyStoreIDGroupingDefect(t *testing.T) {
	rows := candidates([]offer{priced("bread", 1, 2), priced("milk", 2, 1)})
	broken := legacyReference(rows, false)
	corrected := legacyReference(rows, true)
	if len(broken.StorePlans) != 1 || len(broken.StorePlans[0].Items) != 2 {
		t.Fatal("frozen legacy code must demonstrate the zero-key collapse")
	}
	if len(corrected.StorePlans) != 2 || corrected.StorePlans[0].Store.ID != 1 || corrected.StorePlans[1].Store.ID != 2 ||
		broken.TotalCost != corrected.TotalCost || broken.Savings != corrected.Savings {
		t.Fatal("only the corrected grouping should differ")
	}
}

func TestOptimizeUsesPackagedArtifact(t *testing.T) {
	t.Setenv("SWISS_OPTIMIZER_BIN", "")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(exe), "swiss-optimizer")); err != nil {
		t.Fatalf("test must run beside normal native build artifact (make api-test): %v", err)
	}
	got := assertPlan(t, []offer{priced("bread", 1, 2), priced("milk", 2, 1)})
	if len(got.StorePlans) != 2 || got.StorePlans[0].Items[0].Name != "bread" || got.StorePlans[1].Items[0].Name != "milk" {
		t.Fatal("multi-store regression: chosen offers must reach their actual scanned stores")
	}
}

func TestOptimizeHTTPParity(t *testing.T) {
	sameName := []offer{priced("milk", 1, 3), priced("milk", 2, 1.5), priced("milk", 3, 7)}
	sameName[1].productID = 99 // Different linked products still collapse by name.
	mixed := []offer{priced("milk", 1, 2), priced("bread", 2, 1)}
	mixed[0].currency = "EUR" // Preserve the existing CHF label/numeric policy.
	for name, rows := range map[string][]offer{
		"multi-store-worst":                {priced("milk", 2, 1.2), priced("milk", 1, 3), priced("bread", 1, 2), priced("bread", 2, 5)},
		"repeated-name-different-products": sameName,
		"first-tie":                        {priced("bread", 8, 2), priced("bread", 1, 2)},
		"reversed-tie":                     {priced("bread", 1, 2), priced("bread", 8, 2)},
		"unicode":                          {priced("Äpfel 🛒", 4, 0.5), priced("é", 4, 2), priced("e\u0301", 4, 3), priced("茶", 1, 1)},
		"empty":                            nil,
		"no-linked-offers":                 {},
		"empty-name":                       {priced("", 1, 0)},
		"mixed-currency":                   mixed,
		"negative-prices":                  {priced("bread", 1, -0.125), priced("bread", 2, 1.005)},
		"sum-before-rounding":              {priced("a", 1, 0.004), priced("b", 1, 0.004), priced("c", 2, 0.004)},
		"savings-before-rounding":          {priced("a", 1, 0.124), priced("a", 2, 0.249)},
	} {
		t.Run(name, func(t *testing.T) { assertPlan(t, rows) })
	}
}

func TestOptimizeJSONContract(t *testing.T) {
	status, body := optimizeHTTP(t, fixtureFor(nil))
	if status != 200 || string(body) != `{"total_cost":0,"currency":"CHF","savings":0,"store_plans":[]}` {
		t.Fatalf("empty arrays and schema changed: %d %s", status, body)
	}
	status, body = optimizeHTTP(t, fixtureFor([]offer{priced("milk", 1, 2)}))
	if status != 200 {
		t.Fatalf("unexpected status %d", status)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil || len(raw) != 4 {
		t.Fatalf("unexpected top-level fields: %s", body)
	}
	for _, omitted := range []string{`"unit"`, `"website_url"`, `"logo_url"`, `"product_id"`, `"quantity"`} {
		if strings.Contains(string(body), omitted) {
			t.Fatalf("unexpected formerly omitted field %s: %s", omitted, body)
		}
	}
}

func TestOptimizeGoRoundBoundaries(t *testing.T) {
	for _, value := range []float64{0.005, 0.015, 0.125, 1.005, 2.675, -0.005, -0.125, 45035996273704.95} {
		for _, direction := range []float64{math.Inf(-1), value, math.Inf(1)} {
			x := math.Nextafter(value, direction)
			t.Run(fmt.Sprintf("%x", math.Float64bits(x)), func(t *testing.T) {
				assertPlan(t, []offer{priced("rounding", 1, x)})
			})
		}
	}
}

func TestOptimizeSeededParity(t *testing.T) {
	rng := rand.New(rand.NewSource(773))
	for sample := 0; sample < 40; sample++ {
		t.Run(fmt.Sprintf("batch-%d", sample), func(t *testing.T) {
			var rows []offer
			for item := 0; item < 100; item++ {
				name := fmt.Sprintf("synthetic-%02d-Ä", rng.Intn(40))
				base := float64(rng.Intn(20_000)-1000) / 1000
				for store := 1; store <= 3; store++ {
					row := priced(name, store, base+float64(store)*0.125)
					row.productID = int64(item + 1)
					rows = append(rows, row)
				}
			}
			assertPlan(t, rows)
		})
	}
}

func TestOptimizeLargeBatch(t *testing.T) {
	rows := make([]offer, optimizer.MaxCandidates)
	for i := range rows {
		rows[i] = priced(fmt.Sprintf("item-%06d", i), i%13+1, float64(i%17)/100)
	}
	assertPlan(t, rows)
}

func TestOptimizeScanErrorsStillSkipRows(t *testing.T) {
	good := priced("milk", 1, 2)
	f := fixtureFor([]offer{priced("bad", 2, 1), good})
	f.rows[0][1] = "invalid-product-id"
	status, body := optimizeHTTP(t, f)
	want, err := json.Marshal(legacyReference(candidates([]offer{good}), true))
	if err != nil {
		t.Fatal(err)
	}
	if status != 200 || string(body) != string(want) {
		t.Fatalf("scan error policy changed: %d %s", status, body)
	}
}

func TestOptimizeHTTPFailures(t *testing.T) {
	for _, name := range []string{"query", "row-stream", "row-close", "missing-artifact", "missing-artifact-empty", "native-overflow", "invalid-price", "field-limit"} {
		t.Run(name, func(t *testing.T) {
			f := fixtureFor([]offer{priced("milk", 1, 2)})
			switch name {
			case "query":
				f.queryError = errors.New("synthetic query failure")
			case "row-stream":
				f.streamError = errors.New("synthetic row failure")
			case "row-close":
				f.closeError = errors.New("synthetic close failure")
			case "missing-artifact", "missing-artifact-empty":
				t.Setenv("SWISS_OPTIMIZER_BIN", filepath.Join(t.TempDir(), "missing"))
				if name == "missing-artifact-empty" {
					f = fixtureFor(nil)
				}
			case "native-overflow":
				f.rows[0][5] = math.MaxFloat64
			case "invalid-price":
				f.rows[0][5] = math.Inf(1)
			case "field-limit":
				f.rows[0][0] = strings.Repeat("x", optimizer.MaxFieldBytes+1)
			}
			status, body := optimizeHTTP(t, f)
			if status != 500 || string(body) != `{"error":"failed to optimize"}` {
				t.Fatalf("expected contained generic failure, got %d %s", status, body)
			}
		})
	}
}
