package optimizer

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/LLRHook/swiss-grocery/services/api/internal/models"
)

func artifact(t *testing.T) string {
	t.Helper()
	path, err := executable()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("required normal build artifact missing (run make api-test): %v", err)
	}
	return path
}

func frame(name []byte, price float64) []byte {
	var b bytes.Buffer
	b.WriteString("SGOP0001")
	_ = binary.Write(&b, binary.LittleEndian, uint32(1))
	_ = binary.Write(&b, binary.LittleEndian, uint32(len(name)))
	b.Write(name)
	_ = binary.Write(&b, binary.LittleEndian, int64(7))
	_ = binary.Write(&b, binary.LittleEndian, price)
	return b.Bytes()
}

func TestNativeRejectsInvalidProtocol(t *testing.T) {
	path := artifact(t)
	cases := map[string][]byte{
		"empty":       nil,
		"version":     []byte("SGOP9999\x00\x00\x00\x00"),
		"truncated":   frame([]byte("x"), 1)[:20],
		"trailing":    append(frame([]byte("x"), 1), 0),
		"utf8":        frame([]byte{0xff}, 1),
		"nan":         frame([]byte("x"), math.NaN()),
		"infinity":    frame([]byte("x"), math.Inf(1)),
		"name-limit":  frame(bytes.Repeat([]byte{'x'}, MaxFieldBytes+1), 1),
		"input-limit": bytes.Repeat([]byte{'x'}, MaxInputBytes+1),
		"count-limit": []byte("SGOP0001\xff\xff\xff\xff"),
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), Timeout)
			defer cancel()
			stdout, err := execute(ctx, path, bytes.NewReader(input))
			var exit *exec.ExitError
			if !errors.Is(err, ErrNative) || !errors.As(err, &exit) || exit.ExitCode() != 2 || len(stdout) != 0 {
				t.Fatalf("expected native protocol failure without response, got err=%v stdout=%d", err, len(stdout))
			}
		})
	}
}

func TestNativeIdleProcessIsKilledAndReaped(t *testing.T) {
	// The real executable waits for EOF; withhold it to exercise an actual kill,
	// rather than replacing the native runner with a sleeping test program.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	stdin, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	defer writer.Close()
	start := time.Now()
	_, err = execute(ctx, artifact(t), stdin)
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > time.Second {
		t.Fatalf("expected deadline to terminate and reap native process: err=%v elapsed=%v", err, time.Since(start))
	}
}

func TestBatchHonorsDeadline(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	var batch Batch
	_, err := batch.Optimize(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline, got %v", err)
	}
}

func TestMissingArtifactFailsWithoutFallback(t *testing.T) {
	t.Setenv("SWISS_OPTIMIZER_BIN", filepath.Join(t.TempDir(), "missing-optimizer"))
	var batch Batch
	_, err := batch.Optimize(context.Background())
	if !errors.Is(err, ErrNative) || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected missing-artifact error even for empty batch, got %v", err)
	}
}

func TestRelativeArtifactPathRejected(t *testing.T) {
	t.Setenv("SWISS_OPTIMIZER_BIN", "./swiss-optimizer")
	var batch Batch
	if _, err := batch.Optimize(context.Background()); !errors.Is(err, ErrNative) {
		t.Fatalf("expected native configuration error, got %v", err)
	}
}

func TestNativeCalculationFailureHasNoFallback(t *testing.T) {
	artifact(t)
	var batch Batch
	if err := batch.Add(Candidate{Name: "overflow", Store: models.Store{ID: 1}, Price: math.MaxFloat64}); err != nil {
		t.Fatal(err)
	}
	if _, err := batch.Optimize(context.Background()); !errors.Is(err, ErrNative) {
		t.Fatalf("expected real native overflow failure, got %v", err)
	}
}

func TestBatchBounds(t *testing.T) {
	for name, row := range map[string]Candidate{
		"nan":          {Price: math.NaN()},
		"infinity":     {Price: math.Inf(-1)},
		"invalid-utf8": {Name: string([]byte{0xff})},
		"name":         {Name: strings.Repeat("x", MaxFieldBytes+1)},
		"metadata":     {Store: models.Store{Name: strings.Repeat("x", MaxFieldBytes+1)}},
	} {
		t.Run(name, func(t *testing.T) {
			var batch Batch
			if err := batch.Add(row); !errors.Is(err, ErrInput) {
				t.Fatalf("expected input rejection, got %v", err)
			}
		})
	}
	t.Run("row-limit", func(t *testing.T) {
		var batch Batch
		for i := 0; i < MaxCandidates; i++ {
			if err := batch.Add(Candidate{Name: "x"}); err != nil {
				t.Fatal(err)
			}
		}
		if err := batch.Add(Candidate{Name: "x"}); !errors.Is(err, ErrInput) {
			t.Fatalf("expected row limit, got %v", err)
		}
	})
	t.Run("byte-limit", func(t *testing.T) {
		var batch Batch
		row := Candidate{Name: strings.Repeat("x", MaxFieldBytes)}
		for i := 0; i < 256; i++ {
			err := batch.Add(row)
			if errors.Is(err, ErrInput) {
				return
			}
			if err != nil {
				t.Fatal(err)
			}
		}
		t.Fatal("expected input byte limit")
	})
}

func TestOutputLimitCancelsProcessContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := boundedBuffer{limit: 4, cancel: cancel}
	if _, err := w.Write([]byte("1234")); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("5")); err == nil || !w.exceeded || ctx.Err() != context.Canceled || w.data.Len() != 4 {
		t.Fatal("output limit must retain bounded data and cancel")
	}
}

type responseGroup struct {
	id      int64
	indices []uint32
}

func response(total, savings float64, groups ...responseGroup) []byte {
	var b bytes.Buffer
	b.WriteString("SGRS0001")
	_ = binary.Write(&b, binary.LittleEndian, total)
	_ = binary.Write(&b, binary.LittleEndian, savings)
	_ = binary.Write(&b, binary.LittleEndian, uint32(len(groups)))
	for _, group := range groups {
		_ = binary.Write(&b, binary.LittleEndian, group.id)
		_ = binary.Write(&b, binary.LittleEndian, float64(1))
		_ = binary.Write(&b, binary.LittleEndian, uint32(len(group.indices)))
		for _, index := range group.indices {
			_ = binary.Write(&b, binary.LittleEndian, index)
		}
	}
	return b.Bytes()
}

func TestRejectsMalformedNativeOutput(t *testing.T) {
	group := func(id int64, indices ...uint32) responseGroup { return responseGroup{id, indices} }
	rows := []Candidate{{Name: "a", Store: models.Store{ID: 1}}, {Name: "b", Store: models.Store{ID: 2}}}
	valid := response(2, 0, group(1, 0), group(2, 1))
	cases := map[string][]byte{
		"header":       []byte("SGRS9999"),
		"truncated":    valid[:len(valid)-1],
		"trailing":     append(append([]byte{}, valid...), 0),
		"nan":          response(math.NaN(), 0, group(1, 0), group(2, 1)),
		"inf":          response(0, math.Inf(1), group(1, 0), group(2, 1)),
		"missing":      response(1, 0, group(1, 0)),
		"index":        response(2, 0, group(1, 20)),
		"store":        response(2, 0, group(7, 0)),
		"duplicate":    response(2, 0, group(1, 0, 0)),
		"group-order":  response(2, 0, group(2, 1), group(1, 0)),
		"empty-group":  response(2, 0, group(1)),
		"output-limit": make([]byte, MaxOutputBytes+1),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := decode(data, rows); !errors.Is(err, ErrProtocol) {
				t.Fatalf("expected protocol rejection, got %v", err)
			}
		})
	}
	if _, err := decode(valid, rows); err != nil {
		t.Fatalf("valid control: %v", err)
	}
}
