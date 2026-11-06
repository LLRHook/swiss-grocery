// Package optimizer transports a bounded batch to the required Rust executable.
// Selection, store grouping, totals and savings live exclusively in Rust.
package optimizer

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"time"
	"unicode/utf8"

	"github.com/LLRHook/swiss-grocery/services/api/internal/models"
)

const (
	MaxCandidates  = 100_000
	MaxInputBytes  = 16 << 20
	MaxFieldBytes  = 64 << 10
	MaxOutputBytes = 4 << 20
	Timeout        = 2 * time.Second
)

var (
	ErrInput     = errors.New("optimizer input exceeds limits or is invalid")
	ErrProtocol  = errors.New("invalid optimizer response")
	ErrNative    = errors.New("native optimizer failed")
	processSlots = make(chan struct{}, 4)
)

type Candidate struct {
	Name  string
	Store models.Store
	Price float64
}

// Batch limits retained SQL data as well as the smaller native request frame.
type Batch struct {
	rows  []Candidate
	bytes int
}

func (b *Batch) Add(row Candidate) error {
	size := 20
	for _, field := range []string{row.Name, row.Store.Name, row.Store.Slug, row.Store.WebsiteURL, row.Store.LogoURL} {
		if len(field) > MaxFieldBytes || !utf8.ValidString(field) {
			return ErrInput
		}
		size += len(field)
	}
	if len(b.rows) >= MaxCandidates || b.bytes+size+12 > MaxInputBytes || !finite(row.Price) {
		return ErrInput
	}
	b.bytes += size
	b.rows = append(b.rows, row)
	return nil
}

func executable() (string, error) {
	if path := os.Getenv("SWISS_OPTIMIZER_BIN"); path != "" {
		if !filepath.IsAbs(path) {
			return "", fmt.Errorf("%w: executable path must be absolute", ErrNative)
		}
		return path, nil
	}
	path, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("%w: locate executable", ErrNative)
	}
	return filepath.Join(filepath.Dir(path), "swiss-optimizer"), nil
}

func (b *Batch) Optimize(ctx context.Context) (models.OptimizedPlan, error) {
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	select {
	case processSlots <- struct{}{}:
		defer func() { <-processSlots }()
	case <-ctx.Done():
		return models.OptimizedPlan{}, ctx.Err()
	}
	path, err := executable()
	if err != nil {
		return models.OptimizedPlan{}, err
	}
	var request bytes.Buffer
	request.WriteString("SGOP0001")
	put(&request, uint32(len(b.rows)))
	for _, row := range b.rows {
		put(&request, uint32(len(row.Name)))
		request.WriteString(row.Name)
		put(&request, int64(row.Store.ID))
		put(&request, row.Price)
	}
	data, err := execute(ctx, path, &request)
	if err != nil {
		return models.OptimizedPlan{}, err
	}
	return decode(data, b.rows)
}

// execute is also exercised with a real pipe withholding EOF, to verify that
// deadline cancellation terminates the actual native program while it blocks.
func execute(ctx context.Context, path string, request io.Reader) ([]byte, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stdout := boundedBuffer{limit: MaxOutputBytes, cancel: cancel}
	stderr := boundedBuffer{limit: 4096, cancel: cancel}
	cmd := exec.CommandContext(ctx, path)
	cmd.Env = []string{"RUST_BACKTRACE=0"}
	cmd.Stdin = request
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.WaitDelay = 100 * time.Millisecond
	err := cmd.Run() // Always waits/reaps, including after CommandContext kills it.
	if stdout.exceeded || stderr.exceeded {
		return nil, fmt.Errorf("%w: output limit", ErrNative)
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNative, err)
	}
	return stdout.data.Bytes(), nil
}

func put(w io.Writer, value any) {
	// The only caller writes fixed-width numbers to an in-memory bytes.Buffer.
	_ = binary.Write(w, binary.LittleEndian, value)
}

type boundedBuffer struct {
	data     bytes.Buffer
	limit    int
	exceeded bool
	cancel   context.CancelFunc
}

func (w *boundedBuffer) Write(p []byte) (int, error) {
	if len(p) > w.limit-w.data.Len() {
		w.exceeded = true
		w.cancel()
		return 0, errors.New("optimizer output limit")
	}
	return w.data.Write(p)
}

func finite(f float64) bool { return !math.IsNaN(f) && !math.IsInf(f, 0) }

func decode(data []byte, rows []Candidate) (models.OptimizedPlan, error) {
	result := models.OptimizedPlan{Currency: "CHF", StorePlans: []models.StorePlan{}}
	fail := func() (models.OptimizedPlan, error) { return models.OptimizedPlan{}, ErrProtocol }
	if len(data) < 28 || len(data) > MaxOutputBytes || string(data[:8]) != "SGRS0001" {
		return fail()
	}
	r := bytes.NewReader(data[8:])
	read := func(v any) bool { return binary.Read(r, binary.LittleEndian, v) == nil }
	var groups uint32
	if !read(&result.TotalCost) || !read(&result.Savings) || !read(&groups) ||
		!finite(result.TotalCost) || !finite(result.Savings) || int(groups) > len(rows) {
		return fail()
	}
	seen := make(map[string]bool)
	var previousID int64
	for group := uint32(0); group < groups; group++ {
		var id int64
		var count uint32
		plan := models.StorePlan{Items: []models.OptimizedItem{}}
		if !read(&id) || !read(&plan.Subtotal) || !read(&count) || !finite(plan.Subtotal) ||
			count == 0 || int(count) > len(rows) || (group > 0 && id <= previousID) {
			return fail()
		}
		previousID = id
		var previousName string
		for item := uint32(0); item < count; item++ {
			var index uint32
			if !read(&index) || int(index) >= len(rows) {
				return fail()
			}
			row := rows[index]
			if int64(row.Store.ID) != id || seen[row.Name] || (item > 0 && row.Name <= previousName) {
				return fail()
			}
			if item == 0 {
				plan.Store = row.Store
			}
			previousName = row.Name
			seen[row.Name] = true
			plan.Items = append(plan.Items, models.OptimizedItem{Name: row.Name, Price: row.Price})
		}
		result.StorePlans = append(result.StorePlans, plan)
	}
	if r.Len() != 0 {
		return fail()
	}
	for _, row := range rows {
		if !seen[row.Name] { // Detect missing selections without reimplementing them.
			return fail()
		}
	}
	return result, nil
}
